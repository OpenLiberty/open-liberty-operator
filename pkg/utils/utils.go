package utils

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// Utility methods specific to Open Liberty and its configuration

var log = logf.Log.WithName("openliberty_utils")

//Constant Values
const serviceabilityMountPath = "/serviceability"
const ssoEnvVarPrefix = "SEC_SSO_"

// Validate if the OpenLibertyApplication is valid
func Validate(olapp *openlibertyv1beta1.OpenLibertyApplication) (bool, error) {
	// Serviceability validation
	if olapp.GetServiceability() != nil {
		if olapp.GetServiceability().GetVolumeClaimName() == "" && olapp.GetServiceability().GetSize() == "" {
			return false, fmt.Errorf("Invalid input for Serviceability. Specify one of the following: spec.serviceability.size, spec.serviceability.volumeClaimName")
		}
		if olapp.GetServiceability().GetVolumeClaimName() == "" {
			if _, err := resource.ParseQuantity(olapp.GetServiceability().GetSize()); err != nil {
				return false, fmt.Errorf("validation failed: cannot parse '%v': %v", olapp.GetServiceability().GetSize(), err)
			}
		}
	}

	return true, nil
}

func requiredFieldMessage(fieldPaths ...string) string {
	return "must set the field(s): " + strings.Join(fieldPaths, ",")
}

// ExecuteCommandInContainer Execute command inside a container in a pod through API
func ExecuteCommandInContainer(config *rest.Config, podName, podNamespace, containerName string, command []string) (string, error) {

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create Clientset")
		return "", fmt.Errorf("Failed to create Clientset: %v", err.Error())
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(podNamespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Command:   command,
		Container: containerName,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("Encountered error while creating Executor: %v", err.Error())
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return stderr.String(), fmt.Errorf("Encountered error while running command: %v ; Stderr: %v ; Error: %v", command, stderr.String(), err.Error())
	}

	return stderr.String(), nil
}

// CustomizeLibertyEnv adds configured env variables appending configured liberty settings
func CustomizeLibertyEnv(pts *corev1.PodTemplateSpec, la *openlibertyv1beta1.OpenLibertyApplication) {
	// ENV variables have already been set, check if they exist before setting defaults
	targetEnv := []corev1.EnvVar{
		{Name: "WLP_LOGGING_CONSOLE_LOGLEVEL", Value: "info"},
		{Name: "WLP_LOGGING_CONSOLE_SOURCE", Value: "message,accessLog,ffdc,audit"},
		{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "json"},
	}

	if la.GetServiceability() != nil {
		targetEnv = append(targetEnv,
			corev1.EnvVar{Name: "IBM_HEAPDUMPDIR", Value: serviceabilityMountPath},
			corev1.EnvVar{Name: "IBM_COREDIR", Value: serviceabilityMountPath},
			corev1.EnvVar{Name: "IBM_JAVACOREDIR", Value: serviceabilityMountPath},
		)
	}

	envList := pts.Spec.Containers[0].Env
	for _, v := range targetEnv {
		if _, found := findEnvVar(v.Name, envList); !found {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, v)
		}
	}
}

// findEnvVars checks if the environment variable is already present
func findEnvVar(name string, envList []corev1.EnvVar) (*corev1.EnvVar, bool) {
	for i, val := range envList {
		if val.Name == name {
			return &envList[i], true
		}
	}
	return nil, false
}

// CreateServiceabilityPVC creates PersistentVolumeClaim for Serviceability
func CreateServiceabilityPVC(instance *openlibertyv1beta1.OpenLibertyApplication) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name + "-serviceability",
			Namespace:   instance.Namespace,
			Labels:      instance.GetLabels(),
			Annotations: instance.GetAnnotations(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(instance.GetServiceability().GetSize()),
				},
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
				corev1.ReadWriteOnce,
			},
		},
	}
}

// ConfigureServiceability setups the shared-storage for serviceability
func ConfigureServiceability(pts *corev1.PodTemplateSpec, la *openlibertyv1beta1.OpenLibertyApplication) {
	if la.GetServiceability() != nil {
		name := "serviceability"

		foundVolumeMount := false
		for _, v := range pts.Spec.Containers[0].VolumeMounts {
			if v.Name == name {
				foundVolumeMount = true
			}
		}

		if !foundVolumeMount {
			vm := corev1.VolumeMount{
				Name:      name,
				MountPath: serviceabilityMountPath,
			}
			pts.Spec.Containers[0].VolumeMounts = append(pts.Spec.Containers[0].VolumeMounts, vm)
		}

		foundVolume := false
		for _, v := range pts.Spec.Volumes {
			if v.Name == name {
				foundVolume = true
			}
		}

		if !foundVolume {
			claimName := la.Name + "-serviceability"
			if la.Spec.Serviceability.VolumeClaimName != "" {
				claimName = la.Spec.Serviceability.VolumeClaimName
			}
			vol := corev1.Volume{
				Name: name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			}
			pts.Spec.Volumes = append(pts.Spec.Volumes, vol)
		}
	}
}

func normalizeEnvVariableName(name string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToUpper(name))
}

// getValue returns value for string
func getValue(v interface{}) string {
	switch v.(type) {
	case string:
		return v.(string)
	case bool:
		return strconv.FormatBool(v.(bool))
	default:
		return ""
	}
}

// createEnvVarSSO creates an environment variable for SSO
func createEnvVarSSO(loginID string, envSuffix string, value interface{}) *corev1.EnvVar {
	return &corev1.EnvVar{
		Name:  ssoEnvVarPrefix + loginID + envSuffix,
		Value: getValue(value),
	}
}

// CustomizeEnvSSO Process the configuration for SSO login providers
func CustomizeEnvSSO(pts *corev1.PodTemplateSpec, instance *openlibertyv1beta1.OpenLibertyApplication, client client.Client, isOpenShift bool) error {
	const ssoSecretNameSuffix = "-olapp-sso"
	const autoregFragment = "-autoreg-"
	secretName := instance.GetName() + ssoSecretNameSuffix
	ssoSecret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}, ssoSecret)
	if err != nil {
		return errors.Wrapf(err, "Secret for Single sign-on (SSO) was not found. Create a secret named %q in namespace %q with the credentials for the login providers you selected in application image.", secretName, instance.GetNamespace())
	}

	ssoEnv := []corev1.EnvVar{}

	var secretKeys []string
	for k := range ssoSecret.Data { //ranging over a map returns it's keys.
		if strings.Contains(k, autoregFragment) { // skip -autoreg-
			continue
		}
		secretKeys = append(secretKeys, k)
	}
	sort.Strings(secretKeys)

	// append all the values in the secret into the env vars.
	for _, k := range secretKeys {
		ssoEnv = append(ssoEnv, corev1.EnvVar{
			Name: ssoEnvVarPrefix + normalizeEnvVariableName(k),
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ssoSecret.GetName(),
					},
					Key: k,
				},
			},
		})
	}

	// append all the values in the spec into the env vars.
	sso := instance.Spec.SSO
	if sso.MapToUserRegistry != nil {
		ssoEnv = append(ssoEnv, *createEnvVarSSO("", "MAPTOUSERREGISTRY", *sso.MapToUserRegistry))
	}

	if sso.RedirectToRPHostAndPort != "" {
		ssoEnv = append(ssoEnv, *createEnvVarSSO("", "REDIRECTTORPHOSTANDPORT", sso.RedirectToRPHostAndPort))
	}

	if sso.Github != nil && sso.Github.Hostname != "" {
		ssoEnv = append(ssoEnv, *createEnvVarSSO("", "GITHUB_HOSTNAME", sso.Github.Hostname))
	}

	ssoSecretUpdates := make(map[string][]byte)
	for _, oidcClient := range sso.OIDC {
		id := strings.ToUpper(oidcClient.ID)
		if id == "" {
			id = "OIDC"
		}
		if oidcClient.DiscoveryEndpoint != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_DISCOVERYENDPOINT", oidcClient.DiscoveryEndpoint))
		}
		if oidcClient.GroupNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_GROUPNAMEATTRIBUTE", oidcClient.GroupNameAttribute))
		}
		if oidcClient.UserNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_USERNAMEATTRIBUTE", oidcClient.UserNameAttribute))
		}
		if oidcClient.DisplayName != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_DISPLAYNAME", oidcClient.DisplayName))
		}
		if oidcClient.UserInfoEndpointEnabled != nil {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_USERINFOENDPOINTENABLED", *oidcClient.UserInfoEndpointEnabled))
		}
		if oidcClient.RealmNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_REALMNAMEATTRIBUTE", oidcClient.RealmNameAttribute))
		}
		if oidcClient.Scope != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_SCOPE", oidcClient.Scope))
		}
		if oidcClient.TokenEndpointAuthMethod != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_TOKENENDPOINTAUTHMETHOD", oidcClient.TokenEndpointAuthMethod))
		}
		if oidcClient.HostNameVerificationEnabled != nil {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_HOSTNAMEVERIFICATIONENABLED", *oidcClient.HostNameVerificationEnabled))
		}
		// if no clientId specified for this provider, try auto-registration
		clientName := oidcClient.ID
			if clientName == "" {
			    clientName = "oidc"
			}
		clientId := string(ssoSecret.Data[clientName + "-clientId"])
		clientSecret := string(ssoSecret.Data[clientName + "-clientSecret"])

		if isOpenShift && clientId == "" {
			theRoute := &routev1.Route{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, theRoute)
			if err != nil {
				// if route is unavailable, we want to let reconciliation proceed so it will be created.
				// Update status of the instance so reconcilation will be triggered again.
				b := false
				instance.Status.RouteAvailable = &b
				logf.Log.WithName("utils").Info("CustomizeEnvSSO waiting for route to become available for provider " + clientName + ", requeue")
				return nil
			}

			// route available, we don't have a client id and secret yet, go get one
			prefix := strings.ToLower(id) + autoregFragment
			buf := string(ssoSecret.Data[prefix+"insecureTLS"])
			insecure := buf == "true" || buf == "TRUE"
			regData := RegisterData{
				DiscoveryURL:            oidcClient.DiscoveryEndpoint,
				RouteURL:                "https://" + theRoute.Spec.Host,
				RedirectToRPHostAndPort: sso.RedirectToRPHostAndPort,
				InitialAccessToken:      string(ssoSecret.Data[prefix+"initialAccessToken"]),
				InitialClientId:         string(ssoSecret.Data[prefix+"initialClientId"]),
				InitialClientSecret:     string(ssoSecret.Data[prefix+"initialClientSecret"]),
				GrantTypes:              string(ssoSecret.Data[prefix+"grantTypes"]),
				Scopes:                  string(ssoSecret.Data[prefix+"scopes"]),
				InsecureTLS:             insecure,
				ProviderId:              clientName,
			}

			clientId, clientSecret, err = RegisterWithOidcProvider(regData)
			if err != nil {
				return errors.Wrapf(err, "Error occured during registration with OIDC for provider " + clientName)
			}
            
			ssoSecretUpdates[clientName + autoregFragment + "RegisteredOidcClientId"] = []byte(clientId)
			ssoSecretUpdates[clientName + autoregFragment + "RegisteredOidcSecret"] = []byte(clientSecret)
			ssoSecretUpdates[clientName + "-clientId"] = []byte(clientId)
			ssoSecretUpdates[clientName + "-clientSecret"] = []byte(clientSecret)

			b := true
			instance.Status.RouteAvailable = &b
		} // end auto-reg
	} // end for

	if len(ssoSecretUpdates) > 0 { // performant: do all the secret udpates at once
		_, err = controllerutil.CreateOrUpdate(context.TODO(), client, ssoSecret, func() error {
			for key, value := range ssoSecretUpdates {
				ssoSecret.Data[key] = value
			}
			return nil
		})

		if err != nil {
			return errors.Wrapf(err, "Error occured when updating SSO secret")
		}
	}

	for _, oauth2Client := range sso.Oauth2 {
		id := strings.ToUpper(oauth2Client.ID)
		if id == "" {
			id = "OAUTH2"
		}
		if oauth2Client.TokenEndpoint != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_TOKENENDPOINT", oauth2Client.TokenEndpoint))
		}
		if oauth2Client.AuthorizationEndpoint != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_AUTHORIZATIONENDPOINT", oauth2Client.AuthorizationEndpoint))
		}
		if oauth2Client.GroupNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_GROUPNAMEATTRIBUTE", oauth2Client.GroupNameAttribute))
		}
		if oauth2Client.UserNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_USERNAMEATTRIBUTE", oauth2Client.UserNameAttribute))
		}
		if oauth2Client.DisplayName != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_DISPLAYNAME", oauth2Client.DisplayName))
		}
		if oauth2Client.RealmNameAttribute != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_REALMNAMEATTRIBUTE", oauth2Client.RealmNameAttribute))
		}
		if oauth2Client.RealmName != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_REALMNAME", oauth2Client.RealmName))
		}
		if oauth2Client.Scope != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_SCOPE", oauth2Client.Scope))
		}
		if oauth2Client.TokenEndpointAuthMethod != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_TOKENENDPOINTAUTHMETHOD", oauth2Client.TokenEndpointAuthMethod))
		}
		if oauth2Client.AccessTokenHeaderName != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_ACCESSTOKENHEADERNAME", oauth2Client.AccessTokenHeaderName))
		}
		if oauth2Client.AccessTokenRequired != nil {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_ACCESSTOKENREQUIRED", *oauth2Client.AccessTokenRequired))
		}
		if oauth2Client.AccessTokenSupported != nil {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_ACCESSTOKENSUPPORTED", *oauth2Client.AccessTokenSupported))
		}
		if oauth2Client.UserApiType != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_USERAPITYPE", oauth2Client.UserApiType))
		}
		if oauth2Client.UserApi != "" {
			ssoEnv = append(ssoEnv, *createEnvVarSSO(id, "_USERAPI", oauth2Client.UserApi))
		}
	}

	secretRev := corev1.EnvVar{
		Name:  "SSO_SECRET_REV",
		Value: ssoSecret.ResourceVersion}
	ssoEnv = append(ssoEnv, secretRev)

	envList := pts.Spec.Containers[0].Env
	for _, v := range ssoEnv {
		if _, found := findEnvVar(v.Name, envList); !found {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, v)
		}
	}
	return nil
}
