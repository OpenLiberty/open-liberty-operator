package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	LibertyProxyName = "liberty-proxy"
)

func GetLibertyProxyClient(client client.Client, operatorNamespace string, operatorShortName string) (*http.Client, error) {
	caCertSecret := &corev1.Secret{}
	caCertSecret.Name = operatorShortName + "-ca-tls"
	caCertSecret.Namespace = operatorNamespace
	err := client.Get(context.TODO(), types.NamespacedName{Name: caCertSecret.Name, Namespace: caCertSecret.Namespace}, caCertSecret)
	if err != nil {
		return nil, err
	}
	caCerts := x509.NewCertPool()
	caCerts.AppendCertsFromPEM(caCertSecret.Data["ca.crt"])
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCerts,
			},
		},
	}
	return httpClient, nil
}

func GetLibertyProxy(operatorNamespace string, client *http.Client, cmd string, args ...string) (*http.Response, error) {
	proxyServiceName := LibertyProxyName
	proxyServiceNamespace := operatorNamespace
	cmdList := ""
	if len(args) > 0 {
		cmdList += "?"
		cmdList += strings.Join(args, "&")
	}
	requestURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:9443/proxy/%s%s", proxyServiceName, proxyServiceNamespace, cmd, cmdList)
	return client.Get(requestURL)
}
