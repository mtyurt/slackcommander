package slackcommander

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const slacktoken = "testtoken"

type simpleTextCommand func(args CommandArgs) string

func simpleCommandDef(f func(args CommandArgs) string) CommandDef {
	return CommandDef{Handler: func(args CommandArgs) (*CommandResponse, error) {
		resp := SimpleTextResponse(f(args))
		return &resp, nil
	}}
}
func TestRegisterCommandHandler(t *testing.T) {
	m := SlackMux{Token: slacktoken}
	m.RegisterCommandHandler("samplecommand", simpleCommandDef(func(args CommandArgs) string {
		return args.User + " customresponse"
	}))
	slackCmd, ok := m.commandMap["samplecommand"]
	if !ok {
		t.Fatal("slackCmd is not registered")
	}
	if resp, err := slackCmd.Handler(CommandArgs{User: "user1"}); resp.Attachments[0].Text != "user1 customresponse" || err != nil {
		t.Fatal("invocation failed, response:", resp, ", error:", err)
	}
}

func TestAsyncMessage(t *testing.T) {
	//when
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer ts.Close()
	params := make(url.Values)
	params.Add("token", slacktoken)
	params.Add("user_name", "tarik")
	params.Add("text", "")
	params.Set("response_url", ts.URL)
	m := &SlackMux{Token: slacktoken}
	m.RegisterCommandHandler("start", simpleCommandDef(func(args CommandArgs) string {
		return args.User + " customresponse"
	}))
	m.RegisterCommandHandler("error", CommandDef{Handler: func(args CommandArgs) (*CommandResponse, error) {
		return nil, errors.New(args.User + " error")
	}})
	table := map[string]string{
		"start":          "Command received, wait for it...",
		"error":          "Command received, wait for it...",
		"invalidcommand": "invalidcommand is not a valid command.",
		"":               "Provide a command",
	}
	for k, v := range table {
		//setup
		params.Set("text", k)
		//test
		resp, resultChan := callHandlerWithParams(m, params)
		//then
		if resp != v {
			t.Fatal("wrong response: " + resp + ", expected: " + v)
		}
		<-resultChan
		t.Log(k + " is passed")
	}
}

func TestAsyncHttpCall(t *testing.T) {
	//when
	params := make(url.Values)
	params.Add("token", slacktoken)
	params.Add("user_name", "tarik")
	params.Add("text", "")
	m := &SlackMux{Token: slacktoken}
	m.RegisterCommandHandler("start", simpleCommandDef(func(args CommandArgs) string {
		return args.User + " customresponse"
	}))
	m.RegisterCommandHandler("error", CommandDef{Handler: func(args CommandArgs) (*CommandResponse, error) {
		return nil, errors.New(args.User + " error")
	}})
	table := []struct {
		msg            string
		expected       string
		expectedResult bool
	}{
		{"start", "tarik customresponse", true},
		{"error", "something went wrong - tarik error", true}}
	for _, v := range table {
		//setup
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Command %s body read failed, %v", v.expected, err)
			}
			var resp CommandResponse
			err = json.Unmarshal(body, &resp)
			if err != nil {
				t.Fatalf("Command %s body [%s] unmarshal failed, %v", v.expected, string(body), err)
			}

			if resp.Attachments[0].Text != v.expected {
				t.Fatal("wrong response: " + resp.Attachments[0].Text + ", expected: " + v.expected)
			}
		}))
		defer ts.Close()
		params.Set("text", v.msg)
		params.Set("response_url", ts.URL)
		//test
		firstResp, resultChan := callHandlerWithParams(m, params)
		if firstResp != "Command received, wait for it..." {
			t.Fatal("First response is wrong:" + firstResp)
		}

		if actual := <-resultChan; actual != v.expectedResult {
			t.Fatalf("HTTP call channel return is wrong, expected: %v, actual: %v", v.expectedResult, actual)
		}
		t.Log(v.expected + " is passed")
	}
}

func TestWithDefaultHandler(t *testing.T) {
	//when
	params := make(url.Values)
	params.Add("token", slacktoken)
	params.Add("user_name", "tarik")
	params.Add("text", "")
	m := &SlackMux{Token: slacktoken}
	m.RegisterCommandHandler("start", simpleCommandDef(func(args CommandArgs) string {
		return args.User + " customresponse"
	}))
	m.RegisterCommandHandler("error", CommandDef{Handler: func(args CommandArgs) (*CommandResponse, error) {
		return nil, errors.New(args.User + " error")
	}})
	m.RegisterDefaultHandler(simpleCommandDef(func(args CommandArgs) string {
		return "default handler"
	}))

	table := map[string]string{
		"start":          "tarik customresponse",
		"error":          "something went wrong - tarik error",
		"invalidcommand": "default handler",
		"":               "default handler",
	}
	for k, v := range table {
		//setup
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Command %s body read failed, %v", k, err)
			}
			var resp CommandResponse
			err = json.Unmarshal(body, &resp)
			if err != nil {
				t.Fatalf("Command %s body [%s] unmarshal failed, %v", k, string(body), err)
			}

			if resp.Attachments[0].Text != v {
				t.Fatal("wrong response: " + resp.Attachments[0].Text + ", expected: " + v)
			}
		}))
		defer ts.Close()
		params.Set("text", k)
		params.Set("response_url", ts.URL)
		//test
		_, resultChan := callHandlerWithParams(m, params)
		if <-resultChan != true {
			t.Fatal("Channel returned false")
		}
		t.Log(k + " is passed")
	}
}

func TestSendingResponse(t *testing.T) {
	m := &SlackMux{Token: slacktoken}

	table := []struct {
		msg      string
		err      error
		expected string
	}{
		{"response1", nil, "response1"},
		{"", errors.New("error1"), "something went wrong - error1"}}

	for _, v := range table {
		//setup
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Command %v body read failed, %v", v, err)
			}
			var resp CommandResponse
			err = json.Unmarshal(body, &resp)
			if err != nil {
				t.Fatalf("Command %v body [%s] unmarshal failed, %v", v, string(body), err)
			}

			if resp.Attachments[0].Text != v.expected {
				t.Fatal("wrong response: " + resp.Attachments[0].Text + ", expected: " + v.expected)
			}
		}))
		defer ts.Close()

		resultChan := make(chan bool, 1)
		m.handleCommand(CommandDef{Handler: func(args CommandArgs) (*CommandResponse, error) {
			resp := SimpleTextResponse(v.msg)
			return &resp, v.err
		}}, CommandArgs{ResponseURL: ts.URL}, resultChan)
		if <-resultChan != true {
			t.Fatal("Channel returned false")
		}
		t.Logf("%v is passed", v.expected)
	}
}
func callHandlerWithParams(mux *SlackMux, params url.Values) (string, chan bool) {
	recorder := httptest.NewRecorder()
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/cal"},
		Form:   params,
	}
	resultChan := mux.slackHandlerWrapper(recorder, req)
	return recorder.Body.String(), resultChan
}
