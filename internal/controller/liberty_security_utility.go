package controller

import (
	"fmt"
	"os/exec"
)

const SECURITY_UTILITY_BINARY = "/opt/ol/wlp/bin/securityUtility"
const SECURITY_UTILITY_ENCODE = "encode"
const SECURITY_UTILITY_CREATE_LTPA_KEYS = "createLTPAKeys"
const SECURITY_UTILITY_OUTPUT_FOLDER = "/opt/ol/wlp/output"

func encode(password string, passwordKey *string) ([]byte, error) {
	params := []string{}
	params = append(params, SECURITY_UTILITY_ENCODE)
	params = append(params, fmt.Sprintf("--encoding=%s", "aes"))
	if passwordKey != nil && len(*passwordKey) > 0 {
		params = append(params, fmt.Sprintf("--key=%s", *passwordKey))
	}
	params = append(params, password)
	return callSecurityUtility(params)
}

func createLTPAKeys(password string, passwordKey *string) ([]byte, error) {
	params := []string{}
	params = append(params, SECURITY_UTILITY_CREATE_LTPA_KEYS)
	tmpFileName := fmt.Sprintf("ltpa-keys-%s.keys", password)
	params = append(params, fmt.Sprintf("--file=%s/%s", SECURITY_UTILITY_OUTPUT_FOLDER, tmpFileName))
	params = append(params, fmt.Sprintf("--passwordEncoding=%s", "aes")) // use aes encoding
	if passwordKey != nil && len(*passwordKey) > 0 {
		params = append(params, fmt.Sprintf("--passwordKey=%s", *passwordKey))
	}
	params = append(params, fmt.Sprintf("--password=%s", password))
	callSecurityUtility(params)

	params = []string{}
	params = append(params, "-c")
	params = append(params, fmt.Sprintf("cat %s | base64", tmpFileName))
	return callCommand("/bin/bash", params)
}

func callSecurityUtility(params []string) ([]byte, error) {
	return callCommand(SECURITY_UTILITY_BINARY, params)
}

func callCommand(binary string, params []string) ([]byte, error) {
	cmd := exec.Command(binary, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return []byte{}, err
	}
	return stdout, nil
}
