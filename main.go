package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

//ProductionHosts should be configured to include all hosts the mobile can connect to
var ProductionHosts Hosts = []Host{
	{name: "pacific", url: "https://pacific.withsempo.com"},
	{name: "demo", url: "https://demo.withsempo.com"},
}

//Host provides the structure for user-configured hosts
type Host struct {
	name string
	url  string
}

//Hosts is a list of all hosts that the mobile app can connect to
type Hosts []Host

//CustomResponse allows for a customisable JSON response to the server
type CustomResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

//CheckForValidAuth checks the provided credentials against a provided host, returning the host-provided json
// if auth is valid, or 'false' if the credentials are not valid
func CheckForValidAuth(host Host, inputBody string) (json []byte, ok bool) {

	var jsonStr = []byte(inputBody)

	endpoint := fmt.Sprintf("%s%s", host.url, "/api/v1/auth/request_api_token/")

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return nil, false
	}

	defer resp.Body.Close()

	if !(resp.StatusCode >= 200 && resp.StatusCode <= 299) {
		return nil, false
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}

	return body, true
}

//ConcurrentRespnse adds concurrency capacity to check for valid auth, using the channel c to return the final json body
// Or setting the waitgroup to done if there is an error
func ConcurrentRespnse(host Host, inputBody string, respCh chan []byte, wg *sync.WaitGroup) {
	respbody, ok := CheckForValidAuth(host, inputBody)
	if !ok {
		wg.Done()
		return
	}

	var f interface{}
	err := json.Unmarshal(respbody, &f)
	if err != nil {
		wg.Done()
		return
	}

	m := f.(map[string]interface{})
	m["host_name"] = host.name

	jsonBod, err := json.Marshal(m)
	if err != nil {
		wg.Done()
		return
	}

	respCh <- jsonBod
}

//HandleLambdaEvent is the main event handler for each auth request
func HandleLambdaEvent(
	event events.APIGatewayProxyRequest,
	hosts Hosts) (events.APIGatewayProxyResponse, error) {

	headers := make(map[string]string)

	headers["Access-Control-Allow-Origin"] = "*"
	headers["Access-Control-Allow-Headers"] = "Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token"

	// The WaitGroup will trigger the waitchannel if all host requests fail
	wg := sync.WaitGroup{}
	waitCh := make(chan struct{})

	// respchan will take the first successful host response
	respCh := make(chan []byte)

	// Launch all host requests concurrently
	for _, h := range hosts {
		wg.Add(1)
		go ConcurrentRespnse(h, event.Body, respCh, &wg)
	}

	// Concurrent goroutine that blocks until all hosts report failure, and then triggers the waitchannel
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	// Either one of the hosts will send a response to the respCh, in which case return that response
	// or waitCh will trigger from all the failed hosts, in which case return a failure
	select {
	case jsonBod := <-respCh:

		// Return the first host that reports a success
		return events.APIGatewayProxyResponse{
			Body:       string(jsonBod),
			StatusCode: 200,
			Headers:    headers,
		}, nil

	case <-waitCh:

		// Wait channel closed, which means all hosts reported a failure
		failResp := CustomResponse{Status: "fail", Message: "Invalid username or password"}
		authFailedBody, _ := json.Marshal(failResp)

		return events.APIGatewayProxyResponse{
			Body:       string(authFailedBody),
			StatusCode: 200,
			Headers:    headers,
		}, nil
	}
}

//PartialedHandleLambdaEvent shapes the event handler by injecting a host list and providing a context argument
func PartialedHandleLambdaEvent(
	ctx context.Context,
	event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return HandleLambdaEvent(event, ProductionHosts)
}

func main() {
	lambda.Start(PartialedHandleLambdaEvent)
}
