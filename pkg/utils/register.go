package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	corev1 "k8s.io/api/core/v1"
	
    
)

type RegisterData struct {
	DiscoveryURL            string
	RouteURL                string
	RedirectToRPHostAndPort string
	ProviderId              string
	Scopes                  string
	GrantTypes              string
	InitialAccessToken      string
	InitialClientId         string
	InitialClientSecret     string
}


func RegisterWithOidcProvider(regData RegisterData, regSecret *corev1.Secret)(string, string, error){
	populateFromSecret(&regData, regSecret)
	return doRegister(regData)
}

func populateFromSecret(regData *RegisterData, regSecret *corev1.Secret){
	// retrieve iat, clientId, clientSecret, grant-types, scopes
	regData.InitialAccessToken = string(regSecret.Data["InitialAccessToken"])
	regData.InitialClientId = string(regSecret.Data["InitialClientId"])
	regData.InitialClientSecret = string(regSecret.Data["InitialClientSecret"])
	regData.GrantTypes = string(regSecret.Data["GrantTypes"])
	regData.Scopes = string(regSecret.Data["Scopes"])
}


// register with oidc provider and create a new client.  return the new client id and client secret, or an error.
func doRegister(rdata RegisterData) (string, string, error) {
	// process:
	//  1) call the provider's discovery endpoint to find the token and registration urls.
	//  2) If we do not have an initial access token, 
	//  2.5) Use supplied clientId and secret in a Client Credentials grant to obtain an access token.
	//  3) Use the access token to register and obtain a new client id and secret.

	registrationURL, tokenURL, err := getURLs(rdata.DiscoveryURL)
	if err != nil {
		return "", "", err
	}

	// ICI: if we don't have initial token, use client and secret to go get one.
	var token = rdata.InitialAccessToken
	if token == "" {
		rtoken, err := requestAccessToken(rdata, tokenURL)
		if err != nil {
			return "", "", err
		}
		token = rtoken
	}

	fmt.Println("token: " + rdata.InitialAccessToken )

	registrationRequestJson := buildRegistrationRequestJson(rdata)
	fmt.Println("request: " + registrationRequestJson)
	registrationResponse, err := sendHTTPRequest(registrationRequestJson, registrationURL, "POST", "", token)
	if err != nil {
		return "", "", err
	}

	// extract id and secret from body
	id, secret, err := parseRegistrationResponseJson(registrationResponse)
	if err != nil {
		return "", "", err
	}
	return id, secret, nil
}

func requestAccessToken(rdata RegisterData, tokenURL string) (string, error) {
	tokenRequestContent := "grant_type=client_credentials&scope=openid"
	tokenResponse, err := sendHTTPRequest(tokenRequestContent, tokenURL, "POST", rdata.InitialClientId, rdata.InitialClientSecret)
	if err != nil {
		return "", err
	}
	token, err := parseTokenResponse(tokenResponse)
	if err != nil {
		return "", err
	}
	return token, nil

}

// parse token response and return token
func parseTokenResponse(respJson string) (string, error) {
	type token struct {
		Access_token string
	}
	var cdata token
	err := json.Unmarshal([]byte(respJson), &cdata)
	if err != nil {
		return "", errors.New("Error parsing token response: " + err.Error() + " Data: " + respJson)
	}
	return cdata.Access_token, nil
}

// parse the response and return the client id and client secret
func parseRegistrationResponseJson(respJson string) (string, string, error) {
	type idsecret struct {
		Client_id     string
		Client_secret string
	}

	var cdata idsecret
	err := json.Unmarshal([]byte(respJson), &cdata)
	if err != nil {
		return "", "", errors.New("Error parsing registration response: " + err.Error() + " Data: " + respJson)
	}
	return cdata.Client_id, cdata.Client_secret, nil
}

// build the JSON for the client registration request. Form the redirectURL from the route URL.
func buildRegistrationRequestJson(rdata RegisterData) string {
	
	now := time.Now()
	sysClockMillisec := now.UnixNano() / 1000000
	//	rhsso will not accept a supplied value for client_id, so leave a comment in the name
	var clientName = "createdByOpenLibertyOperator-" + strconv.FormatInt(sysClockMillisec, 10)

	return "{" +
		"\"client_name\":\"" + clientName + "\"," +
		"\"grant_types\":["+ getGrantTypes(rdata) + "]," +
		"\"scope\":\"" + getScopes(rdata) + "\"," + 
		"\"redirect_uris\":[\"" + getRedirectUri(rdata) + "\"]}"
}

func getScopes(rdata RegisterData)(string){
	if (rdata.Scopes == ""){
		return "\"openid profile\"";
	}
	fmt.Println(rdata.Scopes)
	var result=""
	gts := strings.Split(rdata.Scopes,",")
	for _,gt := range gts{
		fmt.Println("gt " + gt)
		result +=  strings.Trim(gt, " ") + " "
	}
	return strings.TrimSuffix(result," ")
}

func getGrantTypes(rdata RegisterData)(string){
	if (rdata.GrantTypes == ""){
		return "\"authorization_code\",\"refresh_token\"";
	}
	fmt.Println(rdata.GrantTypes)
	var result=""
	gts := strings.Split(rdata.GrantTypes,",")
	for _,gt := range gts{
		fmt.Println("gt " + gt)
		result +=  "\"" + strings.Trim(gt, " ") + "\"" + ","
	}
	return strings.TrimSuffix(result,",")
}

func getRedirectUri(rdata RegisterData)(string){
	providerId := rdata.ProviderId
	if (providerId == ""){
		providerId = "oidc"
	}
	suffix := "/ibm/api/social-login/redirect/"+providerId
	if(rdata.RedirectToRPHostAndPort !=""){
		return rdata.RedirectToRPHostAndPort + suffix
	}
	return rdata.RouteURL + suffix
}

// retrieve the registration and token URLs from the provider's discovery URL.
// return an error if we don't get back two valid url's.
// todo: more error checking needed to make that true?
func getURLs(discoveryURL string) (string, string, error) {
	discoveryResult, err := sendHTTPRequest("", discoveryURL, "GET", "", "")
	if err != nil {
		return "", "", err
	}

	type regEp struct {
		Registration_endpoint string
	}

	type tokenEp struct {
		Token_endpoint string
	}

	var regdata regEp
	var tokendata tokenEp
	err = json.Unmarshal([]byte(discoveryResult), &regdata)
	if err != nil {
		return "", "", errors.New("Error unmarshaling data from discovery endpoint: " + err.Error() + " Data: " + discoveryResult)
	}
	err = json.Unmarshal([]byte(discoveryResult), &tokendata)
	if err != nil {
		return "", "", errors.New("Error unmarshaling data from discovery endpoint: " + err.Error() + " Data: " + discoveryResult)
	}

	return regdata.Registration_endpoint, tokendata.Token_endpoint, nil
}

// Send an http(s)  request.  return response body and error.
// content to send can be an empty string. Json will be detected. Method should be GET or POST.
// if id is set, send id and passwordOrToken as basic auth header, otherwise send token as bearer auth header.
// If error occurs, body will be "error".
func sendHTTPRequest(content string, URL string, method string, id string, passwordOrToken string) (string, error) {

	client := &http.Client{
		Timeout: time.Second * 20,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	var requestBody = []byte(content)

	request, err := http.NewRequest(method, URL, bytes.NewBuffer(requestBody))
	if strings.HasPrefix(content, "{") {
		request.Header.Set("Content-type", "application/json")
		request.Header.Set("Accept", "application/json")
	} else {
		request.Header.Set("Content-type", "application/x-www-form-urlencoded")
	}

	if id != "" {
		request.SetBasicAuth(id, passwordOrToken)
	} else {
		if passwordOrToken != "" {
			request.Header.Set("Authorization", "Bearer "+passwordOrToken)
		}
	}

	const errorStr = "error"
	var errorMsgPreamble = "Error occurred accessing " + URL + ": "
	if err != nil {
		return errorStr, errors.New(errorMsgPreamble + err.Error())
	}

	response, err := client.Do(request)
	if response == nil {
		return errorStr, errors.New(errorMsgPreamble + err.Error()) // bad hostname, can't connect, etc.
	}
	defer response.Body.Close()

	if err != nil {
		return errorStr, errors.New(errorMsgPreamble + err.Error()) // timeout, conn reset, etc.
	}

	respBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errorStr, errors.New(errorMsgPreamble + err.Error())
	}
	respString := string(respBytes)

	// a successful registration usually has a 201 response code.
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return errorStr, errors.New(errorMsgPreamble + response.Status + ". " + respString + ". Data sent was: " + content)
	}
	return respString, nil
}
