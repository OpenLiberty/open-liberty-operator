package controller

import (
	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	corev1 "k8s.io/api/core/v1"
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

// var (
// 	libertyProxyName = "liberty-proxy"
// )

// func (r *ReconcileOpenLiberty) getLibertyProxyClient(operatorNamespace string) (*http.Client, error) {
// 	caCertSecret := &corev1.Secret{}
// 	caCertSecret.Name = OperatorShortName + "-ca-tls"
// 	caCertSecret.Namespace = operatorNamespace
// 	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: caCertSecret.Name, Namespace: caCertSecret.Namespace}, caCertSecret)
// 	if err != nil {
// 		return nil, err
// 	}
// 	caCerts := x509.NewCertPool()
// 	caCerts.AppendCertsFromPEM(caCertSecret.Data["ca.crt"])
// 	client := &http.Client{
// 		Transport: &http.Transport{
// 			TLSClientConfig: &tls.Config{
// 				RootCAs: caCerts,
// 			},
// 		},
// 	}
// 	return client, nil
// }

// func (r *ReconcileOpenLiberty) getLibertyProxy(operatorNamespace string, client *http.Client, cmd string, args ...string) (*http.Response, error) {
// 	proxyServiceName := libertyProxyName
// 	proxyServiceNamespace := operatorNamespace
// 	cmdList := ""
// 	if len(args) > 0 {
// 		cmdList += "?"
// 		cmdList += strings.Join(args, "&")
// 	}
// 	requestURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:9443/proxy/%s%s", proxyServiceName, proxyServiceNamespace, cmd, cmdList)
// 	return client.Get(requestURL)
// }

func (r *ReconcileOpenLiberty) reconcileLibertyProxy(operatorNamespace string) (string, error) {
	// ServiceAccount
	proxyServiceAccount := &corev1.ServiceAccount{}
	proxyServiceAccount.Name = OperatorShortName + "-" + lutils.LibertyProxyName
	proxyServiceAccount.Namespace = operatorNamespace
	if err := r.CreateOrUpdate(proxyServiceAccount, nil, func() error {
		return nil
	}); err != nil {
		return "Failed to reconcile ServiceAccount for the Liberty proxy", err
	}

	// Proxy
	proxy := &olv1.OpenLibertyApplication{}
	proxy.Name = lutils.LibertyProxyName
	proxy.Namespace = operatorNamespace
	expose := false
	manageTLS := true
	if err := r.CreateOrUpdate(proxy, nil, func() error {
		proxy.Spec.Expose = &expose
		if proxy.Spec.NetworkPolicy == nil {
			trueBool := true
			proxy.Spec.NetworkPolicy = &olv1.OpenLibertyApplicationNetworkPolicy{Disable: &trueBool}
		}
		// if proxy.Spec.NetworkPolicy.NamespaceLabels == nil {
		// 	proxy.Spec.NetworkPolicy.NamespaceLabels = &map[string]string{
		// 		"kubernetes.io/metadata.name": "openshift-operators",
		// 	}
		// }
		// if proxy.Spec.NetworkPolicy.FromLabels == nil {
		// 	proxy.Spec.NetworkPolicy.FromLabels = &map[string]string{
		// 		"app.kubernetes.io/name": "open-liberty-operator",
		// 	}
		// }
		proxy.Spec.ApplicationImage = "liberty-proxy-1-ol" // TODO: update
		if proxy.Spec.Service == nil {
			proxy.Spec.Service = &olv1.OpenLibertyApplicationService{}
		}
		proxy.Spec.Service.Port = 9443
		proxy.Spec.ManageTLS = &manageTLS
		if proxy.Spec.ServiceAccount == nil {
			proxy.Spec.ServiceAccount = &olv1.OpenLibertyApplicationServiceAccount{}
		}
		proxy.Spec.ServiceAccount.Name = &proxyServiceAccount.Name
		// proxy.Spec.Volumes = []corev1.Volume{
		// 	{
		// 		Name: "proxy-tls",
		// 		VolumeSource: corev1.VolumeSource{
		// 			Secret: &corev1.SecretVolumeSource{
		// 				SecretName: "liberty-proxy-tls",
		// 			},
		// 		},
		// 	},
		// }
		// proxy.Spec.VolumeMounts = []corev1.VolumeMount{
		// 	{
		// 		Name:      "proxy-tls",
		// 		MountPath: "/output/resources/liberty-operator/admission-webhook",
		// 		ReadOnly:  true,
		// 	},
		// }
		return nil
	}); err != nil {
		return "Failed to reconcile the Liberty proxy", err
	}
	return "", nil
}
