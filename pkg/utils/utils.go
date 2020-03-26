package utils

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	oputils "github.com/application-stacks/runtime-component-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
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
func CustomizeEnvSSO(pts *corev1.PodTemplateSpec, instance *openlibertyv1beta1.OpenLibertyApplication, ssoSecret *corev1.Secret) {

	var secretKeys []string
	for k := range ssoSecret.Data {
		secretKeys = append(secretKeys, k)
	}
	sort.Strings(secretKeys)

	ssoEnv := []corev1.EnvVar{}
	for _, k := range secretKeys {
		ssoEnv = append(ssoEnv, corev1.EnvVar{
			Name: ssoEnvVarPrefix + oputils.normalizeEnvVariableName(k),
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
	}

	for _, oauth2Client := range sso.OAuth2 {
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

	envList := pts.Spec.Containers[0].Env
	for _, v := range ssoEnv {
		if _, found := findEnvVar(v.Name, envList); !found {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, v)
		}
	}
}
