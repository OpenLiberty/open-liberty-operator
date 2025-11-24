package controller

import (
	"fmt"
	"os/exec"

	"github.com/OpenLiberty/open-liberty-operator/utils"
)

const SECURITY_UTILITY_BINARY = "liberty/bin/securityUtility"
const SECURITY_UTILITY_ENCODE = "encode"
const SECURITY_UTILITY_CREATE_LTPA_KEYS = "createLTPAKeys"
const SECURITY_UTILITY_OUTPUT_FOLDER = "liberty/output"

var validPasswordEncodingTypes = []string{"aes", "aes-128"}

func encode(password string, passwordKey *string, passwordEncodingType string) ([]byte, error) {
	params := []string{}
	params = append(params, SECURITY_UTILITY_ENCODE)
	params = append(params, fmt.Sprintf("--encoding=%s", parsePasswordEncodingType(passwordEncodingType)))
	if passwordKey != nil && len(*passwordKey) > 0 {
		params = append(params, fmt.Sprintf("--key=%s", *passwordKey))
	}
	params = append(params, password)
	return callSecurityUtility(params)
}

func createLTPAKeys(password string, passwordKey *string, passwordEncodingType string) ([]byte, error) {
	tmpFileName := fmt.Sprintf("ltpa-keys-%s.keys", utils.GetRandomAlphanumeric(15))
	tmpFilePath := fmt.Sprintf("%s/%s", SECURITY_UTILITY_OUTPUT_FOLDER, tmpFileName)

	// delete possible colliding file
	callDeleteFile(tmpFilePath)

	// mkdir if not exists
	// callMkdir(SECURITY_UTILITY_OUTPUT_FOLDER)

	// create the key
	params := []string{}
	params = append(params, SECURITY_UTILITY_CREATE_LTPA_KEYS)
	params = append(params, fmt.Sprintf("--file=%s", tmpFilePath))
	params = append(params, fmt.Sprintf("--passwordEncoding=%s", parsePasswordEncodingType(passwordEncodingType))) // use aes encoding
	if passwordKey != nil && len(*passwordKey) > 0 {
		params = append(params, fmt.Sprintf("--passwordKey=%s", *passwordKey))
	}
	params = append(params, fmt.Sprintf("--password=%s", password))
	callSecurityUtility(params)

	// read the key
	params = []string{}
	params = append(params, "-c")
	params = append(params, fmt.Sprintf("cat %s | base64", tmpFilePath))
	bytesOut, err := callCommand("/bin/bash", params)

	// delete the key
	callDeleteFile(tmpFilePath)
	return bytesOut, err
}

// Returns the password encoding type to use for Liberty security utility encode and createLTPAKeys. Defaults to "aes" when invalid or undefined.
func parsePasswordEncodingType(passwordEncodingType string) string {
	for _, validType := range validPasswordEncodingTypes {
		if validType == passwordEncodingType {
			return validType
		}
	}
	return "aes"
}

// func callMkdir(folderPath string) {
// 	params := []string{}
// 	params = append(params, "-c")
// 	params = append(params, fmt.Sprintf("mkdir -p %s", folderPath))
// 	callCommand("/bin/bash", params)
// }

func callDeleteFile(filePath string) {
	params := []string{}
	params = append(params, "-c")
	params = append(params, fmt.Sprintf("rm -f %s", filePath))
	callCommand("/bin/bash", params)
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
