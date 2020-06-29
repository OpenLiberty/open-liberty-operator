package utils

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	gherrors "github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	RegistrationURL         string
	InsecureTLS             bool
}

func RegisterWithOidcProvider(regData RegisterData) (string, string, error) {
	return doRegister(regData)
}

// register with oidc provider and create a new client.  return the new client id and client secret, or an error.
func doRegister(rdata RegisterData) (string, string, error) {
	// process:
	//  1) call the provider's discovery endpoint to find the token and registration urls.
	//  2) If we do not have an initial access token,
	//  2.5) Use supplied clientId and secret in a Client Credentials grant to obtain an access token.
	//  3) Use the access token to register and obtain a new client id and secret.

	registrationURL, tokenURL, err := getURLs(rdata.DiscoveryURL, rdata.InsecureTLS, rdata.ProviderId)
	if err != nil {
		return "", "", err
	}
	if tokenURL == "" {
		return "", "", gherrors.New("Provider " + rdata.ProviderId + ": failed to obtain token endpoint from discovery endpoint.")
	}

	// ICI: if we don't have initial token, use client and secret to go get one.
	var token = rdata.InitialAccessToken
	if token == "" && (rdata.InitialClientId == "" || rdata.InitialClientSecret == "") {
	    id := rdata.ProviderId
		return "", "", gherrors.New("Provider " + id + ": registration data for Single sign-on (SSO) is missing required fields," +
			" one or more of " + id + "-autoreg-initialAccessToken, " + id + "-autoreg-initialClientId, or " + id + "-autoreg-initialClientSecret.")
	}
	if token == "" {
		rtoken, err := requestAccessToken(rdata, tokenURL)
		if err != nil {
			return "", "", err
		}
		if rtoken == "" {
			return "", "", gherrors.New("Provider " + rdata.ProviderId + ": failed to obtain access token for registration.")
		}
		rdata.InitialAccessToken = rtoken
	}

	if rdata.RegistrationURL != "" {
		registrationURL = rdata.RegistrationURL
	}
	// registrationURL should be in discovery data but allow it to be supplied manually if not.
	if registrationURL == "" {
		return "", "", gherrors.New("Provider " + rdata.ProviderId + ": failed to obtain registration URL - specify registrationURL in registration data secret.")
	}

	registrationRequestJson := buildRegistrationRequestJson(rdata)

	registrationResponse, err := sendHTTPRequest(registrationRequestJson, registrationURL, "POST", "", rdata.InitialAccessToken, rdata.InsecureTLS, rdata.ProviderId)
	if err != nil {
		return "", "", err
	}

	// extract id and secret from body
	id, secret, err := parseRegistrationResponseJson(registrationResponse, rdata.ProviderId)
	if err != nil {
		return "", "", err
	}
	return id, secret, nil
}

func requestAccessToken(rdata RegisterData, tokenURL string) (string, error) {
	tokenRequestContent := "grant_type=client_credentials&scope=" + getScopes(rdata)
	tokenResponse, err := sendHTTPRequest(tokenRequestContent, tokenURL, "POST", rdata.InitialClientId, rdata.InitialClientSecret, rdata.InsecureTLS, rdata.ProviderId)
	if err != nil {
		return "", err
	}
	token, err := parseTokenResponse(tokenResponse, rdata.ProviderId)
	if err != nil {
		return "", err
	}
	return token, nil

}

// parse token response and return token
func parseTokenResponse(respJson string, providerId string) (string, error) {
	type token struct {
		Access_token string
	}
	var cdata token
	err := json.Unmarshal([]byte(respJson), &cdata)
	if err != nil {
		return "", errors.New("Provider " + providerId + ": error parsing token response: " + err.Error() + " Data: " + respJson)
	}
	return cdata.Access_token, nil
}

// parse the response and return the client id and client secret
func parseRegistrationResponseJson(respJson string, providerId string) (string, string, error) {
	type idsecret struct {
		Client_id     string
		Client_secret string
	}

	var cdata idsecret
	err := json.Unmarshal([]byte(respJson), &cdata)
	if err != nil {
		return "", "", errors.New("Provider " + providerId + ": error parsing registration response: " + err.Error() + " Data: " + respJson)
	}
	return cdata.Client_id, cdata.Client_secret, nil
}

// build the JSON for the client registration request. Form the redirectURL from the route URL.
func buildRegistrationRequestJson(rdata RegisterData) string {
	now := time.Now()
	sysClockMillisec := now.UnixNano() / 1000000
	//	rhsso will not accept a supplied value for client_id, so leave a comment in the name
	clientName := "LibertyOperator-" + strings.Replace(rdata.RouteURL, "https://", "", 1) + "-" +
		strconv.FormatInt(sysClockMillisec, 10)

	// IBM Security Verify needs some special things in the request.
	isvAttribs := ""
	if rdata.InitialClientId != "" {
		isvAttribs = "\"enforce_pkce\":false," +
			"\"all_users_entitled\":true," +
			"\"consent_action\":\"never_prompt\","
	}

	return "{" + isvAttribs +
		"\"client_name\":\"" + clientName + "\"," +
		"\"grant_types\":[" + getGrantTypes(rdata) + "]," +
		"\"scope\":\"" + getScopes(rdata) + "\"," +
		"\"redirect_uris\":[\"" + getRedirectUri(rdata) + "\"]}"
}

func getScopes(rdata RegisterData) string {
	if rdata.Scopes == "" {
		return "openid profile"
	}

	var result = ""
	gts := strings.Split(rdata.Scopes, ",")
	for _, gt := range gts {
		result += strings.Trim(gt, " ") + " "
	}
	return strings.TrimSuffix(result, " ")
}

func getGrantTypes(rdata RegisterData) string {
	if rdata.GrantTypes == "" {
		return "\"authorization_code\",\"refresh_token\""
	}

	var result = ""
	gts := strings.Split(rdata.GrantTypes, ",")
	for _, gt := range gts {
		result += "\"" + strings.Trim(gt, " ") + "\"" + ","
	}
	return strings.TrimSuffix(result, ",")
}

func getRedirectUri(rdata RegisterData) string {
	providerId := rdata.ProviderId
	if providerId == "" {
		providerId = "oidc"
	}
	suffix := "/ibm/api/social-login/redirect/" + providerId
	if rdata.RedirectToRPHostAndPort != "" {
		return rdata.RedirectToRPHostAndPort + suffix
	}
	return rdata.RouteURL + suffix
}

// retrieve the registration and token URLs from the provider's discovery URL.
// return an error if we don't get back two valid url's.
// todo: more error checking needed to make that true?
func getURLs(discoveryURL string, insecureTLS bool, providerId string) (string, string, error) {
	discoveryResult, err := sendHTTPRequest("", discoveryURL, "GET", "", "", insecureTLS, providerId)
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
		return "", "", errors.New("Provider " + providerId + ": error unmarshalling data from discovery endpoint: " + err.Error() + " Data: " + discoveryResult)
	}
	err = json.Unmarshal([]byte(discoveryResult), &tokendata)
	if err != nil {
		return "", "", errors.New("Provider " + providerId + ": error unmarshalling data from discovery endpoint: " + err.Error() + " Data: " + discoveryResult)
	}

	return regdata.Registration_endpoint, tokendata.Token_endpoint, nil
}

// Send an http(s)  request.  return response body and error.
// content to send can be an empty string. Json will be detected. Method should be GET or POST.
// if id is set, send id and passwordOrToken as basic auth header, otherwise send token as bearer auth header.
// If error occurs, body will be "error".
func sendHTTPRequest(content string, URL string, method string, id string, passwordOrToken string, insecureTLS bool, providerId string) (string, error) {

	rootCAPool, _ := x509.SystemCertPool()
	if rootCAPool == nil {
		rootCAPool = x509.NewCertPool()
	}

	if !insecureTLS {
		cert, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt")
		if err != nil {
			return "", errors.New("Error reading TLS certificates: " + err.Error())
		}
		rootCAPool.AppendCertsFromPEM(cert)
	}

	client := &http.Client{
		Timeout: time.Second * 20,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            rootCAPool,
				InsecureSkipVerify: insecureTLS,
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
	var errorMsgPreamble = "Provider " + providerId + ": error occurred communicating with OIDC provider.  URL: " + URL + ": "
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
