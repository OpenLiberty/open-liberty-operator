package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"

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

func (r *ReconcileOpenLiberty) reconcileLibertyProxy(operatorNamespace string, instance *olv1.OpenLibertyApplication) (string, error) {
	// ServiceAccount
	proxyServiceAccount := &corev1.ServiceAccount{}
	proxyServiceAccount.Name = OperatorShortName + "-proxy-sa"
	proxyServiceAccount.Namespace = operatorNamespace
	if err := r.CreateOrUpdate(proxyServiceAccount, nil, func() error {
		return nil
	}); err != nil {
		return "Failed to reconcile ServiceAccount for the Liberty proxy", err
	}

	// ClusterRole
	proxyClusterRole := &rbacv1.ClusterRole{}
	proxyClusterRole.Name = OperatorShortName + "-proxy-clusterrole"
	// proxyClusterRole.Namespace = operatorNamespace
	if err := r.CreateOrUpdate(proxyClusterRole, nil, func() error {
		proxyClusterRole.Rules = []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "get", "update", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts"},
			},
			{
				Verbs:     []string{"create", "get", "update", "list", "watch"},
				APIGroups: []string{"cert-manager.io"},
				Resources: []string{"issuers", "certificates"},
			},
		}
		return nil
	}); err != nil {
		return "Failed to reconcile ClusterRole for the Liberty proxy", err
	}

	// ClusterRoleBinding
	proxyClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	proxyClusterRoleBinding.Name = OperatorShortName + "-proxy-clusterrolebinding"
	if err := r.CreateOrUpdate(proxyClusterRoleBinding, nil, func() error {
		proxyClusterRoleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      proxyServiceAccount.Name,
				Namespace: proxyServiceAccount.Namespace,
			},
		}
		proxyClusterRoleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     proxyClusterRole.Name,
		}
		return nil
	}); err != nil {
		return "Failed to reconcile ClusterRoleBinding for the Liberty proxy", err
	}

	// Proxy
	proxy := &olv1.OpenLibertyApplication{}
	proxy.Name = "liberty-proxy"
	proxy.Namespace = operatorNamespace
	expose := false
	manageTLS := true
	if err := r.CreateOrUpdate(proxy, nil, func() error {
		proxy.Spec.Expose = &expose
		if proxy.Spec.NetworkPolicy == nil {
			proxy.Spec.NetworkPolicy = &olv1.OpenLibertyApplicationNetworkPolicy{}
		}
		if proxy.Spec.NetworkPolicy.NamespaceLabels == nil {
			proxy.Spec.NetworkPolicy.NamespaceLabels = &map[string]string{
				"kubernetes.io/metadata.name": "openshift-operators",
			}
		}
		if proxy.Spec.NetworkPolicy.FromLabels == nil {
			proxy.Spec.NetworkPolicy.FromLabels = &map[string]string{
				"app.kubernetes.io/name": "open-liberty-operator",
			}
		}
		proxy.Spec.ApplicationImage = "liberty-proxy-1-ol"
		if proxy.Spec.Service == nil {
			proxy.Spec.Service = &olv1.OpenLibertyApplicationService{}
		}
		proxy.Spec.Service.Port = 9443
		proxy.Spec.ManageTLS = &manageTLS
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
