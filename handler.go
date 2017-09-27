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
func (mux *SlackMux) initializeMutexAndMap() {
	if mux.commandMu == nil || mux.commandMap == nil {
		mux.commandMu = &sync.Mutex{}
		mux.commandMap = make(map[string]*slackCommand)
	}
}
func (mux *SlackMux) RegisterDefaultHandler(handler SlackCommandHandler, isAsync bool) {
	mux.defaultHandler = &slackCommand{handler: handler, async: isAsync}
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
				fmt.Println("handling formatted response for user", user)
				return slackCmd.formattedResponseHandler(user, commands)
			}, r.FormValue("response_url"))
			return
		} else if slackCmd.async {
			if _, err := fmt.Fprintf(w, "Command received, wait for it..."); err != nil {
				fmt.Printf("Error while sending first response of async command, not aborting", err)
			}
			fmt.Println("Async first response is successful")
			go sendFormattedResponse(func() (*slack.PostMessageParameters, error) {
				text, err := slackCmd.handler(user, commands)
				if err != nil {
					return nil, err
				}
				return &slack.PostMessageParameters{Text: text, Markdown: true}, nil
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
		handlerResp = &slack.PostMessageParameters{Text: "something went wrong - " + err.Error()}
	}
	jsonStr, err := json.Marshal(handlerResp)
	if err != nil {
		fmt.Println("Async call has blown, cannot marshal response data to json")
		return
	}
	fmt.Println("Posting json", string(jsonStr), "to "+responseUrl)
	resp, err := http.Post(responseUrl, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer resp.Body.Close()
	asyncRespBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Async call has blown cannot read response body, response code:", resp.Status, "error:", err.Error())
		return
	}
	if resp.StatusCode >= 400 {
		fmt.Println("Async call has blown, response code:", resp.Status, "response body:", string(asyncRespBody))
		return
	}
	fmt.Println("Async call successful")
}
