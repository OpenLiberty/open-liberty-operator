package utils

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	rcoutils "github.com/application-stacks/runtime-component-operator/utils"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/batch/v1"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Utility methods specific to Open Liberty and its configuration

var log = logf.Log.WithName("openliberty_utils")

// Constant Values
const serviceabilityMountPath = "/serviceability"
const ssoEnvVarPrefix = "SEC_SSO_"
const OperandVersion = "1.3.1"
const ltpaKeysMountPath = "/config/managedLTPA"
const ltpaServerXMLOverridesMountPath = "/config/configDropins/overrides/"
const LTPAServerXMLSuffix = "-managed-ltpa-server-xml"
const ltpaKeysFileName = "ltpa.keys"
const ltpaXMLFileName = "managedLTPA.xml"

// Validate if the OpenLibertyApplication is valid
func Validate(olapp *olv1.OpenLibertyApplication) (bool, error) {
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
func CustomizeLibertyEnv(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication, client client.Client) error {
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

	// If manageTLS is true or not set, and SEC_IMPORT_K8S_CERTS is not set then default it to "true"
	if la.GetManageTLS() == nil || *la.GetManageTLS() {
		targetEnv = append(targetEnv, corev1.EnvVar{Name: "SEC_IMPORT_K8S_CERTS", Value: "true"})
	}

	envList := pts.Spec.Containers[0].Env
	for _, v := range targetEnv {
		if _, found := findEnvVar(v.Name, envList); !found {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, v)
		}
	}

	/*
		if la.GetService() != nil && la.GetService().GetCertificateSecretRef() != nil {
			if err := addSecretResourceVersionAsEnvVar(pts, la, client, *la.GetService().GetCertificateSecretRef(), "SERVICE_CERT"); err != nil {
				return err
			}
		}

		if la.GetRoute() != nil && la.GetRoute().GetCertificateSecretRef() != nil {
			if err := addSecretResourceVersionAsEnvVar(pts, la, client, *la.GetRoute().GetCertificateSecretRef(), "ROUTE_CERT"); err != nil {
				return err
			}
		}
	*/

	return nil
}

func AddSecretResourceVersionAsEnvVar(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication, client client.Client, secretName string, envNamePrefix string) error {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: la.GetNamespace()}, secret)
	if err != nil {
		return errors.Wrapf(err, "Secret %q was not found in namespace %q", secretName, la.GetNamespace())
	}
	pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  envNamePrefix + "_SECRET_RESOURCE_VERSION",
		Value: secret.ResourceVersion})
	return nil
}

func CustomizeLibertyAnnotations(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication) {
	libertyAnnotations := map[string]string{
		"libertyOperator": "Open Liberty",
	}
	pts.Annotations = rcoutils.MergeMaps(pts.Annotations, libertyAnnotations)
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
func CreateServiceabilityPVC(instance *olv1.OpenLibertyApplication) *corev1.PersistentVolumeClaim {
	persistentVolume := &corev1.PersistentVolumeClaim{
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
			},
		},
	}
	if instance.Spec.Serviceability.StorageClassName != "" {
		persistentVolume.Spec.StorageClassName = &instance.Spec.Serviceability.StorageClassName
	}
	return persistentVolume
}

// ConfigureServiceability setups the shared-storage for serviceability
func ConfigureServiceability(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication) {
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

func writeSSOSecretIfNeeded(client client.Client, ssoSecret *corev1.Secret, ssoSecretUpdates map[string][]byte) error {
	var err error = nil
	if len(ssoSecretUpdates) > 0 {
		_, err = controllerutil.CreateOrUpdate(context.TODO(), client, ssoSecret, func() error {
			for key, value := range ssoSecretUpdates {
				ssoSecret.Data[key] = value
			}
			return nil
		})
	}
	return err
}

// CustomizeEnvSSO Process the configuration for SSO login providers
func CustomizeEnvSSO(pts *corev1.PodTemplateSpec, instance *olv1.OpenLibertyApplication, client client.Client, isOpenShift bool) error {
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

		clientName := oidcClient.ID
		if clientName == "" {
			clientName = "oidc"
		}
		// if no clientId specified for this provider, try auto-registration
		clientId := string(ssoSecret.Data[clientName+"-clientId"])
		clientSecret := string(ssoSecret.Data[clientName+"-clientSecret"])

		if isOpenShift && clientId == "" {
			logf.Log.WithName("utils").Info("Processing OIDC registration for id :" + clientName)
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
			prefix := clientName + autoregFragment
			buf := string(ssoSecret.Data[prefix+"insecureTLS"])
			insecure := strings.ToUpper(buf) == "TRUE"
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
				writeSSOSecretIfNeeded(client, ssoSecret, ssoSecretUpdates) // preserve any registrations that succeeded
				return errors.Wrapf(err, "Error occured during registration with OIDC for provider "+clientName)
			}
			logf.Log.WithName("utils").Info("OIDC registration for id: " + clientName + " successful, obtained clientId: " + clientId)
			ssoSecretUpdates[clientName+autoregFragment+"RegisteredOidcClientId"] = []byte(clientId)
			ssoSecretUpdates[clientName+autoregFragment+"RegisteredOidcSecret"] = []byte(clientSecret)
			ssoSecretUpdates[clientName+"-clientId"] = []byte(clientId)
			ssoSecretUpdates[clientName+"-clientSecret"] = []byte(clientSecret)

			b := true
			instance.Status.RouteAvailable = &b
		} // end auto-reg
	} // end for
	err = writeSSOSecretIfNeeded(client, ssoSecret, ssoSecretUpdates)

	if err != nil {
		return errors.Wrapf(err, "Error occured when updating SSO secret")
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

func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func Remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func isVolumeMountFound(pts *corev1.PodTemplateSpec, name string) bool {
	for _, v := range pts.Spec.Containers[0].VolumeMounts {
		if v.Name == name {
			return true
		}
	}
	return false
}

func isVolumeFound(pts *corev1.PodTemplateSpec, name string) bool {
	for _, v := range pts.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

// ConfigureLTPA setups the shared-storage for LTPA keys file generation
func ConfigureLTPA(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication, operatorShortName string) {
	// Mount a volume /config/ltpa to store the ltpa.keys file
	ltpaKeyVolumeMount := GetLTPAKeysVolumeMount(la, ltpaKeysFileName)
	if !isVolumeMountFound(pts, ltpaKeyVolumeMount.Name) {
		pts.Spec.Containers[0].VolumeMounts = append(pts.Spec.Containers[0].VolumeMounts, ltpaKeyVolumeMount)
	}
	if !isVolumeFound(pts, ltpaKeyVolumeMount.Name) {
		vol := corev1.Volume{
			Name: ltpaKeyVolumeMount.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: operatorShortName + "-managed-ltpa",
					Items: []corev1.KeyToPath{{
						Key:  ltpaKeysFileName,
						Path: ltpaKeysFileName,
					}},
				},
			},
		}
		pts.Spec.Volumes = append(pts.Spec.Volumes, vol)
	}

	// Mount a volume /config/configDropins/overrides/ltpa.xml to store the Liberty Server XML
	ltpaXMLVolumeMount := GetLTPAXMLVolumeMount(la, ltpaXMLFileName)
	if !isVolumeMountFound(pts, ltpaXMLVolumeMount.Name) {
		pts.Spec.Containers[0].VolumeMounts = append(pts.Spec.Containers[0].VolumeMounts, ltpaXMLVolumeMount)
	}
	if !isVolumeFound(pts, ltpaXMLVolumeMount.Name) {
		vol := corev1.Volume{
			Name: ltpaXMLVolumeMount.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: operatorShortName + LTPAServerXMLSuffix,
					Items: []corev1.KeyToPath{{
						Key:  ltpaXMLFileName,
						Path: ltpaXMLFileName,
					}},
				},
			},
		}
		pts.Spec.Volumes = append(pts.Spec.Volumes, vol)
	}
}

func CustomizeLTPAServerXML(xmlSecret *corev1.Secret, la *olv1.OpenLibertyApplication, encryptedPassword string) {
	xmlSecret.StringData = make(map[string]string)
	keysFileName := strings.Replace(ltpaKeysMountPath, "/config", "${server.config.dir}", 1)
	xmlSecret.StringData[ltpaXMLFileName] = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<server>\n    <ltpa keysFileName=\"" + keysFileName + "/" + ltpaKeysFileName + "\" keysPassword=\"" + encryptedPassword + "\" />\n</server>"
}

// Returns true if the OpenLibertyApplication leader's state has changed, causing existing LTPA Jobs to need a configuration update, otherwise return false
func IsLTPAJobConfigurationOutdated(job *v1.Job, appLeaderInstance *olv1.OpenLibertyApplication) bool {
	// The Job contains the leader's pull secret
	if appLeaderInstance.GetPullSecret() != nil && *appLeaderInstance.GetPullSecret() != "" {
		ltpaJobHasLeaderPullSecret := false
		for _, objectReference := range job.Spec.Template.Spec.ImagePullSecrets {
			if objectReference.Name == *appLeaderInstance.GetPullSecret() {
				ltpaJobHasLeaderPullSecret = true
			}
		}
		if !ltpaJobHasLeaderPullSecret {
			return true
		}
	}
	if len(job.Spec.Template.Spec.Containers) != 1 {
		return true
	}
	// The Job matches the leader's pull policy
	if job.Spec.Template.Spec.Containers[0].ImagePullPolicy != *appLeaderInstance.GetPullPolicy() {
		return true
	}
	// The Job matches the leader's security context
	if !reflect.DeepEqual(*job.Spec.Template.Spec.Containers[0].SecurityContext, *rcoutils.GetSecurityContext(appLeaderInstance)) {
		return true
	}
	return false
}

func CustomizeLTPAJob(job *v1.Job, la *olv1.OpenLibertyApplication, ltpaSecretName string, serviceAccountName string, ltpaScriptName string) {
	encodingType := "aes" // the password encoding type for securityUtility (one of "xor", "aes", or "hash")
	job.Spec.Template.ObjectMeta.Name = "liberty"
	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            job.Spec.Template.ObjectMeta.Name,
			Image:           la.GetStatus().GetImageReference(),
			ImagePullPolicy: *la.GetPullPolicy(),
			SecurityContext: rcoutils.GetSecurityContext(la),
			Command:         []string{"/bin/bash", "-c"},
			// Usage: /bin/create_ltpa_keys.sh <namespace> <ltpa-secret-name> <securityUtility-encoding>
			Args: []string{ltpaKeysMountPath + "/bin/create_ltpa_keys.sh " + la.GetNamespace() + " " + ltpaSecretName + " " + ltpaKeysFileName + " " + encodingType},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      ltpaScriptName,
					MountPath: ltpaKeysMountPath + "/bin",
				},
			},
		},
	}
	if la.GetPullSecret() != nil && *la.GetPullSecret() != "" {
		job.Spec.Template.Spec.ImagePullSecrets = append(job.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{
			Name: *la.GetPullSecret(),
		})
	}
	job.Spec.Template.Spec.ServiceAccountName = serviceAccountName
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	var number int32
	number = 0777
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: ltpaScriptName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ltpaScriptName,
				},
				DefaultMode: &number,
			},
		},
	})
}

func GetLTPAKeysVolumeMount(la *olv1.OpenLibertyApplication, fileName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "ltpa-keys",
		MountPath: ltpaKeysMountPath + "/" + fileName,
		SubPath:   fileName,
	}
}

func GetLTPAXMLVolumeMount(la *olv1.OpenLibertyApplication, fileName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "ltpa-xml",
		MountPath: ltpaServerXMLOverridesMountPath + fileName,
		SubPath:   fileName,
	}
}

func GetRequiredLabels(name string, instance string) map[string]string {
	requiredLabels := make(map[string]string)
	requiredLabels["app.kubernetes.io/name"] = name
	if instance != "" {
		requiredLabels["app.kubernetes.io/instance"] = instance
	} else {
		requiredLabels["app.kubernetes.io/instance"] = name
	}
	requiredLabels["app.kubernetes.io/managed-by"] = "open-liberty-operator"
	return requiredLabels
}
