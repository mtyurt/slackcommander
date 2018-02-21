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
	Token string

	commandMu      *sync.Mutex
	commandMap     map[string]*slackCommand
	defaultHandler *slackCommand
}

type SlackCommandHandler func(user string, args []string) (string, error)
type CommandHandlerWithFormattedResponse func(user string, args []string) (*slack.PostMessageParameters, error)

type slackCommand struct {
	handler                  SlackCommandHandler
	formattedResponseHandler CommandHandlerWithFormattedResponse
	async                    bool
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
func (mux *SlackMux) RegisterAsyncCommand(command string, handler SlackCommandHandler) {
	mux.registerCommandHandlerWithAsyncOption(command, handler, true)
}
func (mux *SlackMux) RegisterCommand(command string, handler SlackCommandHandler) {
	mux.registerCommandHandlerWithAsyncOption(command, handler, false)
}
func (mux *SlackMux) RegisterCommandWithFormattedResponse(command string, handler CommandHandlerWithFormattedResponse, async bool) {
	mux.initializeMutexAndMap()
	mux.commandMu.Lock()
	mux.commandMap[command] = &slackCommand{handler: nil, async: async, formattedResponseHandler: handler}
	mux.commandMu.Unlock()
}
func (mux *SlackMux) registerCommandHandlerWithAsyncOption(command string, handler SlackCommandHandler, async bool) {
	mux.initializeMutexAndMap()
	mux.commandMu.Lock()
	mux.commandMap[command] = &slackCommand{handler: handler, async: async, formattedResponseHandler: nil}
	mux.commandMu.Unlock()
}
func (mux *SlackMux) ClearCommandHandlers() {
	if initialized := mux.initializeMutexAndMap(); !initialized {
		mux.commandMu.Lock()
		mux.commandMap = make(map[string]*slackCommand)
		mux.commandMu.Unlock()
	}
}
func (mux *SlackMux) initializeMutexAndMap() bool {
	if mux.commandMu == nil || mux.commandMap == nil {
		mux.commandMu = &sync.Mutex{}
		mux.commandMap = make(map[string]*slackCommand)
		return true
	}
	return false
}
func (mux *SlackMux) RegisterDefaultHandler(handler SlackCommandHandler, isAsync bool) {
	mux.defaultHandler = &slackCommand{handler: handler, async: isAsync}
}
func (mux *SlackMux) RegisterDefaultHandlerWithFormattedResponse(handler CommandHandlerWithFormattedResponse, isAsync bool) {
	mux.defaultHandler = &slackCommand{formattedResponseHandler: handler, async: isAsync}
}
func (mux *SlackMux) SlackHandler() func(w http.ResponseWriter, r *http.Request) {
	if mux.Token == "" {
		// we should have a token configured at this point
		panic("Token is missing! Set token first!")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		err := mux.parseRequestAndCheckToken(r)
		if err != nil {
			writeResponseWithBadRequest(&w, err.Error())
			return
		}
		user := r.FormValue("user_name")
		text := r.FormValue("text")
		if text == "" && mux.defaultHandler == nil {
			writeResponseWithBadRequest(&w, "Provide a command")
			return
		}
		commands := strings.Split(text, " ")
		slackCmd, ok := mux.commandMap[commands[0]]
		if !ok {
			if mux.defaultHandler == nil {
				fmt.Fprint(w, commands[0]+" is not a valid command.")
				return
			}
			slackCmd = mux.defaultHandler
		}
		if slackCmd.formattedResponseHandler != nil {
			go sendFormattedResponse(func() (*slack.PostMessageParameters, error) {
				return slackCmd.formattedResponseHandler(user, commands)
			}, r.FormValue("response_url"))
			return
		} else if slackCmd.async {
			if _, err := fmt.Fprintf(w, "Command received, wait for it..."); err != nil {
				fmt.Printf("Error while sending first response of async command, not aborting", err)
			}
			go sendFormattedResponse(func() (*slack.PostMessageParameters, error) {
				text, err := slackCmd.handler(user, commands)
				if err != nil {
					return nil, err
				}
				return &slack.PostMessageParameters{Attachments: []slack.Attachment{slack.Attachment{Text: text}}, Markdown: true}, nil
			}, r.FormValue("response_url"))
			return
		}
		resp, err := slackCmd.handler(user, commands)
		if err != nil {
			writeResponseWithBadRequest(&w, err.Error())
			return
		}
		if _, err := fmt.Fprint(w, resp); err != nil {
			fmt.Println("Error while sending sync response", err)
		}
	}
}
func sendFormattedResponse(commandHandler func() (*slack.PostMessageParameters, error), responseUrl string) {
	handlerResp, err := commandHandler()
	if err != nil {
		fmt.Println("Something went wrong -", err)

		text := "something went wrong - " + err.Error()
		handlerResp = &slack.PostMessageParameters{Attachments: []slack.Attachment{slack.Attachment{Text: text}}}
	}
	err = postResponse(handlerResp, responseUrl)
	if err != nil {
		fmt.Println("Async call has failed:", err.Error())
	}
}
func postResponse(params *slack.PostMessageParameters, url string) error {

	jsonStr, err := json.Marshal(params)
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
