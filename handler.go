package slackcommander

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/nlopes/slack"
)

type SlackMux struct {
	Token                 string
	SkipSlackResponse     bool
	IgnoreSlackFormatting bool

	commandMu      *sync.Mutex
	commandMap     map[string]CommandDef
	defaultHandler *CommandDef
}

type CommandArgs struct {
	User        string
	Command     string
	Args        []string
	FullText    string
	ResponseURL string
	ChannelID   string
	UserID      string
}

func SimpleTextResponse(text string) CommandResponse {
	return CommandResponse{Attachments: []slack.Attachment{slack.Attachment{Text: text}}}
}

type CommandResponse struct {
	Attachments []slack.Attachment `json:"attachments"`
	Markdown    bool               `json:"markdown"`
}

type CommandHandler func(args CommandArgs) (*CommandResponse, error)

type CommandDef struct {
	Handler    CommandHandler
	NoResponse bool
}

func (mux *SlackMux) parseRequestAndCheckToken(r *http.Request) error {
	r.ParseForm()

	if r.FormValue("token") != mux.Token {
		return errors.New("Token invalid, contact an admin")
	}
	return nil
}
func writeResponseWithBadRequest(w *http.ResponseWriter, text string) {
	(*w).WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(*w, text)
}
func (mux *SlackMux) RegisterCommandHandler(command string, cmdDef CommandDef) {
	mux.initializeMutexAndMap()
	mux.commandMu.Lock()
	mux.commandMap[command] = cmdDef
	mux.commandMu.Unlock()
}
func (mux *SlackMux) ClearCommandHandlers() {
	if initialized := mux.initializeMutexAndMap(); !initialized {
		mux.commandMu.Lock()
		mux.commandMap = make(map[string]CommandDef)
		mux.commandMu.Unlock()
		mux.defaultHandler = nil
	}
}
func (mux *SlackMux) initializeMutexAndMap() bool {
	if mux.commandMu == nil || mux.commandMap == nil {
		mux.commandMu = &sync.Mutex{}
		mux.commandMap = make(map[string]CommandDef)
		return true
	}
	return false
}
func (mux *SlackMux) RegisterDefaultHandler(cmdDef CommandDef) {
	mux.defaultHandler = &cmdDef
}

type slackMsgParams struct {
	slack.PostMessageParameters
	Attachments []slack.Attachment `json:"attachments"`
}

func (mux *SlackMux) SlackHandler() func(w http.ResponseWriter, r *http.Request) {
	if mux.Token == "" {
		// we should have a token configured at this point
		panic("Token is missing! Set token first!")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mux.slackHandlerWrapper(w, r)
	}
}

func (mux *SlackMux) slackHandlerWrapper(w http.ResponseWriter, r *http.Request) chan bool {
	resultChan := make(chan bool, 1)
	err := mux.parseRequestAndCheckToken(r)
	if err != nil {
		writeResponseWithBadRequest(&w, err.Error())
		resultChan <- false
		return resultChan
	}
	user := r.FormValue("user_name")
	text := strings.TrimSpace(r.FormValue("text"))
	if mux.IgnoreSlackFormatting {
		text = removeSimpleFormatting(text)
	}
	responseURL := r.FormValue("response_url")
	if text == "" && mux.defaultHandler == nil {
		writeResponseWithBadRequest(&w, "Provide a command")
		resultChan <- false
		return resultChan
	}

	commands := strings.Fields(text)
	commandName := getCommandName(commands)

	if !mux.isCommandValid(commands, commandName) {
		fmt.Fprintf(w, "Command not found: [%s]", commandName)
		resultChan <- true
		return resultChan
	}

	slackCmd := mux.getSlackCmd(commands, commandName)
	cmdArgs := CommandArgs{User: user, Command: commandName, Args: commands, FullText: text, ResponseURL: responseURL, ChannelID: r.FormValue("channel_id"), UserID: r.FormValue("user_id")}

	if _, err := fmt.Fprintf(w, "Command received, wait for it..."); err != nil {
		fmt.Printf("Error while sending first response of async command, not aborting: %v", err)
	}

	go mux.handleCommand(slackCmd, cmdArgs, resultChan)
	return resultChan
}

func (mux *SlackMux) getSlackCmd(commands []string, commandName string) CommandDef {
	if len(commands) > 0 {
		return mux.commandMap[commandName]
	} else {
		return *mux.defaultHandler
	}
}

func (mux *SlackMux) isCommandValid(commands []string, commandName string) bool {
	if len(commands) > 0 {
		_, ok := mux.commandMap[commandName]
		return ok
	} else {
		if mux.defaultHandler == nil {
			return false
		}
	}
	return true
}

func getCommandName(commands []string) string {
	if len(commands) > 0 {
		return commands[0]
	}
	return ""
}

func (mux SlackMux) handleCommand(c CommandDef, args CommandArgs, resultChan chan bool) {
	resp, err := c.Handler(args)

	if c.NoResponse {
		if err != nil {
			fmt.Printf("No response command %v has failed, err: %v\n", args.Command, err)
		}
		return
	}

	var actualResponse CommandResponse
	if err != nil {
		fmt.Println("Something went wrong - ", err)

		text := "something went wrong - " + err.Error()
		actualResponse = CommandResponse{Attachments: []slack.Attachment{slack.Attachment{Text: text}}}
	} else if resp == nil {
		actualResponse = CommandResponse{Attachments: []slack.Attachment{slack.Attachment{Text: "No response - you are not supposed to see this message."}}}
	} else {
		actualResponse = *resp
	}
	err = mux.postResponse(actualResponse, args.ResponseURL)
	if err != nil {
		fmt.Println("Async call has failed:", err.Error())
	}
	resultChan <- err == nil

}
func (mux *SlackMux) postResponse(response CommandResponse, url string) error {
	if mux.SkipSlackResponse {
		fmt.Printf("slackcommander: %v", response)
		return nil
	}

	jsonStr, err := json.Marshal(response)
	if err != nil {
		return errors.New("Marshaling parameters has failed: " + err.Error())
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		return errors.New("Post request has failed: " + err.Error())
	}
	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New("Post request has failed, cannot read response body, response code: " + resp.Status + " error: " + err.Error())
	}
	if resp.StatusCode >= 400 {
		return errors.New("Post message has failed with response code: " + resp.Status + " response body: " + string(respBody))
	}
	return nil
}

func removeSimpleFormatting(str string) string {
	formattingCharacters := []uint8{'*', '~', '_'}
	temp := str
	for len(temp) > 1 {
		if contains(formattingCharacters, temp[0]) && temp[0] == temp[len(temp)-1] {
			temp = temp[1 : len(temp)-1]
		} else {
			break
		}
	}
	return temp
}

func contains(slice []uint8, char uint8) bool {
	for _, t := range slice {
		if char == t {
			return true
		}
	}
	return false
}
