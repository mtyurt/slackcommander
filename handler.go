package slackcommander

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type SlackMux struct {
	Token string

	commandMu      *sync.Mutex
	commandMap     map[string]SlackCommandHandler
	defaultHandler SlackCommandHandler
}

type SlackCommandHandler func(user string, args []string) (string, error)

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
func (mux *SlackMux) RegisterCommand(command string, handler SlackCommandHandler) {
	if mux.commandMu == nil || mux.commandMap == nil {
		mux.commandMu = &sync.Mutex{}
		mux.commandMap = make(map[string]SlackCommandHandler)
	}
	mux.commandMu.Lock()
	mux.commandMap[command] = handler
	mux.commandMu.Unlock()

}
func (mux *SlackMux) RegisterDefaultHandler(handler SlackCommandHandler) {
	mux.defaultHandler = handler
}
func (mux *SlackMux) SlackHandler() func(w http.ResponseWriter, r *http.Request) {
	if mux.Token == "" {
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
		handler, ok := mux.commandMap[commands[0]]
		if !ok {
			if mux.defaultHandler == nil {
				fmt.Fprint(w, commands[0]+" is not a valid command.")
				return
			}
			handler = mux.defaultHandler
		}
		resp, err := handler(user, commands)
		if err != nil {
			writeResponseWithBadRequest(&w, err.Error())
			return
		}
		fmt.Fprint(w, resp)
	}
}
