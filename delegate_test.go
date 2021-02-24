package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

var expectedPath string = "/api/v1/auth/request_api_token/"

func TestCheckForValidAuth(t *testing.T) {

	params := []struct {
		statusCode   int
		json         string
		expectedOk   bool
		expectedBody string
	}{
		{http.StatusAccepted, `{"message": "Some message"}`, true, `{"message": "Some message"}`},
		{http.StatusBadRequest, `{"message": "Some message"}`, false, ""},
	}

	for _, param := range params {

		var URLPath string

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(param.statusCode)

			URLPath = r.URL.Path

			fmt.Fprint(w, param.json)
		}))
		defer ts.Close()

		h := Host{name: "demo", url: ts.URL}

		body, ok := CheckForValidAuth(h, `{"phone": "123456"}`)

		stringified := string(body)

		if URLPath != expectedPath {
			t.Errorf(`Expected URL Path: %s, but recieved %s`, expectedPath, URLPath)
		}

		if ok != param.expectedOk {
			t.Errorf(`Expected ok: %t, but recieved %t`, param.expectedOk, ok)
		}

		if stringified != param.expectedBody {
			t.Errorf(`Expected body: %s, but recieved %s`, param.expectedBody, stringified)
		}
	}

}

type HostGenerator func(string) string

func GoodHostGenerator(url string) string {
	return url
}

func BadHostGenerator(url string) string {
	return "https://some.junk.domain1weofnlwdks"
}

func TestHandler(t *testing.T) {

	params := []struct {
		hostName     string
		generator    HostGenerator
		expectedBody string
	}{
		{"test", GoodHostGenerator, `{"host_name":"test","message":"Some message"}`},
		{"test", BadHostGenerator, `{"status":"fail","message":"Invalid username or password"}`},
	}

	for _, param := range params {

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"message": "Some message"}`)
		}))
		defer ts.Close()

		hosts := Hosts{
			Host{name: param.hostName, url: param.generator(ts.URL)},
			Host{name: "foo", url: "https://another.junk.domain1weofnlwdks"},
		}

		var e = events.APIGatewayProxyRequest{Body: `this_shouldn't_need_to_be_json3223`}

		resp, _ := HandleLambdaEvent(e, hosts)

		if resp.Body != param.expectedBody {
			t.Errorf(`Expected body: %s, but recieved %s`, param.expectedBody, resp.Body)
		}
	}

}

func TestProductionHostURLS(t *testing.T) {
	for _, h := range ProductionHosts {
		_, err := url.ParseRequestURI(h.url)
		if err != nil {
			t.Errorf(`%s is not a valid url`, h.url)
		}

		if strings.Contains(h.url, expectedPath) {
			t.Errorf(`The path %s should not be in the url %s`, expectedPath, h.url)

		}
	}

}
