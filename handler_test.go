package slack

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const slacktoken = "testtoken"

func TestRegisterCommand(t *testing.T) {
	m := SlackMux{Token: slacktoken}
	m.RegisterCommand("samplecommand", func(user string, args []string) (string, error) {
		return user + " customresponse", nil
	})
	handler := m.commandMap["samplecommand"]
	if handler == nil {
		t.Fatal("handler is not registered")
	}
	if resp, err := handler("user1", []string{}); resp != "user1 customresponse" || err != nil {
		t.Fatal("invocation failed, response:", resp, ", error:", err)
	}
}

func TestHandler(t *testing.T) {
	//when
	params := make(url.Values)
	params.Add("token", slacktoken)
	params.Add("user_name", "tarik")
	params.Add("text", "")
	m := &SlackMux{Token: slacktoken}
	m.RegisterCommand("start", func(user string, args []string) (string, error) {
		return user + " customresponse", nil
	})
	m.RegisterCommand("error", func(user string, args []string) (string, error) {
		return "", errors.New(user + " error")
	})
	table := map[string]string{
		"start":          "tarik customresponse",
		"error":          "tarik error",
		"invalidcommand": "invalidcommand is not a valid command.",
		"":               "Provide a command",
	}
	var resp string
	for k, v := range table {
		//setup
		params.Set("text", k)
		//test
		resp = callHandlerWithParams(m, params)
		//then
		if resp != v {
			t.Fatal("wrong response: " + resp + ", expected: " + v)
		}
		t.Log(k + " is passed")
	}
}
func callHandlerWithParams(mux *SlackMux, params url.Values) string {
	recorder := httptest.NewRecorder()
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/cal"},
		Form:   params,
	}
	mux.SlackHandler()(recorder, req)
	return recorder.Body.String()
}
