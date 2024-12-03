package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type SecurityUtilityCreateLTPAKeysResponse struct {
	LTPAKeys    string `json:"ltpa.keys,omitempty"`
	RawPassword string `json:"rawPassword,omitempty"`
	OK          bool   `json:"ok"`
}

type SecurityUtilityEncodeResponse struct {
	Password string `json:"password,omitempty"`
	OK       bool   `json:"ok"`
}

func (r *ReconcileOpenLiberty) getLibertyProxyClient() (*http.Client, error) {
	caCertSecret := &corev1.Secret{}
	caCertSecret.Name = OperatorShortName + "-ca-tls"
	caCertSecret.Namespace = "proxy-test"
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: caCertSecret.Name, Namespace: caCertSecret.Namespace}, caCertSecret)
	if err != nil {
		return nil, err
	}
	caCerts := x509.NewCertPool()
	caCerts.AppendCertsFromPEM(caCertSecret.Data["ca.crt"])
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCerts,
			},
		},
	}
	return client, nil
}

func (r *ReconcileOpenLiberty) getLibertyProxy(instance *olv1.OpenLibertyApplication, client *http.Client, cmd string, args ...string) (*http.Response, error) {
	proxyServiceName := "liberty-proxy-1" // TODO: replace
	proxyServiceNamespace := "proxy-test" // TODO: change
	cmdList := ""
	if len(args) > 0 {
		cmdList += "?"
		cmdList += strings.Join(args, "&")
	}
	requestURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:9443/proxy/%s%s", proxyServiceName, proxyServiceNamespace, cmd, cmdList)
	return client.Get(requestURL)
}
