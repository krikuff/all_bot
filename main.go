package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
)

const dbFilename = "chats.db"

type ChatsDB struct {
	db *sql.DB
}

func NewChatsDB() (*ChatsDB, error) {
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		return nil, err
	}

	return &ChatsDB{db: db}, nil
}

func (self *ChatsDB) Close() {
	self.db.Close()
}

func (self *ChatsDB) GetParticipants(chatId int64) ([]string, error) {
	query := fmt.Sprintf("SELECT member FROM members WHERE id = %v", chatId)
	rows, err := self.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var member string
		if err = rows.Scan(&member); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func (self *ChatsDB) GetInternalChatId(tgChatId int64) (int64, error) {
	query := fmt.Sprintf("SELECT id FROM chats WHERE telegram_id = %d", tgChatId)
	row := self.db.QueryRow(query)

	var internalId int64
	if err := row.Scan(&internalId); err != nil {
		return 0, err
	}
	return internalId, nil
}

type UtilsBot struct {
	bot   *tgbotapi.BotAPI
	db    *ChatsDB
	debug bool
}

func NewUtilsBot(bot *tgbotapi.BotAPI, db *ChatsDB, debug bool) *UtilsBot {
	return &UtilsBot{
		bot:   bot,
		db:    db,
		debug: debug,
	}
}

func (self *UtilsBot) chatIsKnown(tgChatId int64) bool {
	_, err := self.db.GetInternalChatId(tgChatId)
	return err == nil
}

func (self *UtilsBot) notifyAllMembers(tgChatId int64) error {
	internalChatId, err := self.db.GetInternalChatId(tgChatId)
	if err != nil {
		return err
	}

	members, err := self.db.GetParticipants(internalChatId)
	if err != nil {
		return err
	}

	if len(members) == 0 {
		return errors.New("No members found in the chat")
	}

	var format string
	if self.debug {
		format = "%v "
	} else {
		format = "@%v "
	}
	var notifications string
	for i := 0; i < len(members); i += 1 {
		notifications += fmt.Sprintf(format, members[i])
	}

	message := tgbotapi.NewMessage(tgChatId, notifications) // TODO: add beginning of the message
	self.bot.Send(message)

	return nil
}

func (self *UtilsBot) needNotifications(message string) bool {
	return strings.Contains(message, "@all")
}

func (self *UtilsBot) processUpdate(update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}
	if !self.chatIsKnown(update.Message.Chat.ID) {
		return errors.New(fmt.Sprintf("Message in unknown chat with ID %d", update.Message.Chat.ID))
	}

	if self.needNotifications(update.Message.Text) {
		err := self.notifyAllMembers(update.Message.Chat.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *UtilsBot) RunLoop() {
	u := tgbotapi.NewUpdate(0)
	updates := self.bot.GetUpdatesChan(u)
	for update := range updates {
		if err := self.processUpdate(update); err != nil {
			log.Println(err)
		}
	}
}

func main() {
	debugFlag := flag.Bool("d", false, "Disables actual notifications with @")
	flag.Parse()

	token := os.Getenv("BOT_TOKEN")
	botApi, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	chatsDB, err := NewChatsDB()
	if err != nil {
		log.Panic(err)
	}
	defer chatsDB.Close()

	bot := NewUtilsBot(botApi, chatsDB, *debugFlag)
	bot.RunLoop()
}
