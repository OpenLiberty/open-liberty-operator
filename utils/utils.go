package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"math/rand/v2"

	_ "unsafe"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	rcoutils "github.com/application-stacks/runtime-component-operator/utils"
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
	_ "k8s.io/kubectl/pkg/cmd/cp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Utility methods specific to Open Liberty and its configuration

var log = logf.Log.WithName("openliberty_utils")

// Constant Values
const serviceabilityMountPath = "/serviceability"
const ssoEnvVarPrefix = "SEC_SSO_"
const OperandVersion = "1.5.0"

// LTPA constants
const LTPAServerXMLSuffix = "-managed-ltpa-server-xml"
const LTPAServerXMLMountSuffix = "-managed-ltpa-mount-server-xml"
const LTPAKeysFileName = "ltpa.keys"
const LTPAKeysXMLFileName = "managedLTPA.xml"
const LTPAKeysMountXMLFileName = "managedLTPAMount.xml"

// Mount constants
const SecureMountPath = "/output/liberty-operator"
const overridesMountPath = "/config/configDropins/overrides"

// Password encryption constants
const ManagedEncryptionServerXML = "-managed-encryption-server-xml"
const ManagedEncryptionMountServerXML = "-managed-encryption-mount-server-xml"
const PasswordEncryptionKeyRootName = "wlp-password-encryption-key"
const LocalPasswordEncryptionKeyRootName = "olo-wlp-password-encryption-key"
const EncryptionKeyXMLFileName = "encryptionKey.xml"
const EncryptionKeyMountXMLFileName = "encryptionKeyMount.xml"

type LTPAMetadata struct {
	Kind       string
	APIVersion string
	Name       string
	Path       string
	PathIndex  string
}

func (m LTPAMetadata) GetName() string {
	return m.Name
}
func (m LTPAMetadata) GetPath() string {
	return m.Path
}
func (m LTPAMetadata) GetPathIndex() string {
	return m.PathIndex
}
func (m LTPAMetadata) GetKind() string {
	return m.Kind
}
func (m LTPAMetadata) GetAPIVersion() string {
	return m.APIVersion
}

type LTPAMetadataList struct {
	Items []LeaderTrackerMetadata
}

func (ml LTPAMetadataList) GetItems() []LeaderTrackerMetadata {
	return ml.Items
}

type PasswordEncryptionMetadata struct {
	Kind       string
	APIVersion string
	Name       string
	Path       string
	PathIndex  string
}

func (m PasswordEncryptionMetadata) GetName() string {
	return m.Name
}
func (m PasswordEncryptionMetadata) GetPath() string {
	return m.Path
}
func (m PasswordEncryptionMetadata) GetPathIndex() string {
	return m.PathIndex
}
func (m PasswordEncryptionMetadata) GetKind() string {
	return m.Kind
}
func (m PasswordEncryptionMetadata) GetAPIVersion() string {
	return m.APIVersion
}

type PasswordEncryptionMetadataList struct {
	Items []LeaderTrackerMetadata
}

func (ml PasswordEncryptionMetadataList) GetItems() []LeaderTrackerMetadata {
	return ml.Items
}

type LTPAConfig struct {
	Metadata                    *LTPAMetadata
	SecretName                  string
	SecretInstanceName          string
	ConfigSecretName            string
	ConfigSecretInstanceName    string
	ServiceAccountName          string
	JobRequestConfigMapName     string
	ConfigMapName               string
	FileName                    string
	EncryptionKeySecretName     string
	EncryptionKeySharingEnabled bool // true or false
}

type PodInjectorClient interface {
	Connect() error
	CloseConnection()
	PollStatus(scriptName, podName, podNamespace string) string
	StartScript(scriptName, podName, podNamespace, attrs string) bool
}

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

const (
	FlagDelimiterSpace  = " "
	FlagDelimiterEquals = "="
)

func parseFlag(key, value, delimiter string) string {
	return fmt.Sprintf("%s%s%s", key, delimiter, value)
}

func EncodeLinperfAttr(instance *olv1.OpenLibertyPerformanceData) string {
	timespan := instance.GetTimespan()
	interval := instance.GetInterval()
	return fmt.Sprintf("timespan/%d|interval/%d", timespan, interval)
}

func DecodeLinperfAttr(encodedAttr string) map[string]string {
	decodedAttrs := map[string]string{}
	for _, attr := range strings.Split(strings.Trim(encodedAttr, " "), "|") {
		attrArr := strings.Split(attr, "/")
		if len(attrArr) != 2 {
			continue
		}
		attrKey, attrValue := attrArr[0], attrArr[1]
		decodedAttrs[attrKey] = attrValue
	}
	return decodedAttrs
}

func GetLinperfCmd(encodedAttr, podName, podNamespace string) string {
	scriptDir := "/output/helper"
	scriptName := "linperf.sh"

	linperfCmdArgs := []string{fmt.Sprintf("%s/%s", scriptDir, scriptName)}
	outputDir := fmt.Sprintf("/serviceability/%s/%s/performanceData/", podNamespace, podName)
	linperfCmdArgs = append(linperfCmdArgs, parseFlag("--output-dir", outputDir, FlagDelimiterEquals))

	decodedLinperfAttrs := DecodeLinperfAttr(encodedAttr)
	linperfCmdArgs = append(linperfCmdArgs, parseFlag("-s", decodedLinperfAttrs["timespan"], FlagDelimiterSpace))
	linperfCmdArgs = append(linperfCmdArgs, parseFlag("-j", decodedLinperfAttrs["interval"], FlagDelimiterSpace))
	linperfCmdArgs = append(linperfCmdArgs, "--ignore-root")
	linperfCmd := strings.Join(linperfCmdArgs, FlagDelimiterSpace)

	linperfCmdWithPids := fmt.Sprintf("PIDS=$(ls -l /proc/[0-9]*/exe | grep \"/java\" | xargs -L 1 | cut -d ' ' -f9 | cut -d '/' -f 3 ); PIDS_OUT=$(echo $PIDS | tr '\n' ' '); %s \"$PIDS_OUT\"", linperfCmd)
	return linperfCmdWithPids
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
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	fmt.Printf("out: %s\n", stdout.String())

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

func GetSecretLastRotationLabel(la *olv1.OpenLibertyApplication, client client.Client, secretName string, sharedResourceName string) (map[string]string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: la.GetNamespace()}, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "Secret %q was not found in namespace %q", secretName, la.GetNamespace())
	}
	labelKey := GetLastRotationLabelKey(sharedResourceName)
	lastRotationLabel, found := secret.Labels[labelKey]
	if !found {
		return nil, fmt.Errorf("Secret %q does not have label key %q", secretName, labelKey)
	}
	return map[string]string{
		labelKey: string(lastRotationLabel),
	}, nil
}

func GetSecretLastRotationAsLabelMap(la *olv1.OpenLibertyApplication, client client.Client, secretName string, sharedResourceName string) (map[string]string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: la.GetNamespace()}, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "Secret %q was not found in namespace %q", secretName, la.GetNamespace())
	}
	return map[string]string{
		GetLastRotationLabelKey(sharedResourceName): string(secret.Data["lastRotation"]),
	}, nil
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

func GetMaxTime(args ...string) (string, error) {
	maxVal := -1
	for _, arg := range args {
		val := 0
		if arg != "" {
			t, err := strconv.Atoi(arg)
			if err != nil {
				return "", err
			}
			val = t
		}
		if val > maxVal {
			maxVal = val
		}
	}
	if maxVal == -1 {
		return "", fmt.Errorf("no arguments were passed to function GetLatestTimeAsString")
	}
	return strconv.Itoa(maxVal), nil
}

// if time1 >= time2 then return true, otherwise false
func CompareStringTimeGreaterThanOrEqual(time1 string, time2 string) (bool, error) {
	t1, err := strconv.Atoi(time1)
	if err != nil {
		return false, err
	}
	t2, err := strconv.Atoi(time2)
	if err != nil {
		return false, err
	}
	return t1 >= t2, nil
}

func RemovePodTemplateSpecAnnotationByKey(pts *corev1.PodTemplateSpec, annotationKey string) {
	RemoveMapElementByKey(pts.Annotations, annotationKey)
}

func RemoveMapElementByKey(refMap map[string]string, labelKey string) {
	if _, found := refMap[labelKey]; found {
		delete(refMap, labelKey)
	}
}

func AddPodTemplateSpecAnnotation(pts *corev1.PodTemplateSpec, annotation map[string]string) {
	pts.Annotations = rcoutils.MergeMaps(pts.Annotations, annotation)
}

func CustomizeLibertyAnnotations(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication) {
	libertyAnnotations := map[string]string{
		"libertyOperator": "Open Liberty",
	}
	AddPodTemplateSpecAnnotation(pts, libertyAnnotations)
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
			Resources: corev1.VolumeResourceRequirements{
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
				return errors.Wrapf(err, "Error occured during registration with OIDC for provider %s", clientName)
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

func LocalObjectReferenceContainsName(list []corev1.LocalObjectReference, name string) bool {
	for _, v := range list {
		if v.Name == name {
			return true
		}
	}
	return false
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

func ConfigurePasswordEncryption(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication, operatorShortName string, passwordEncryptionMetadata *PasswordEncryptionMetadata) {
	// Mount a volume /output/liberty-operator/encryptionKey.xml to store the Liberty Password Encryption Key
	MountSecretAsVolume(pts, operatorShortName+ManagedEncryptionServerXML+passwordEncryptionMetadata.Name, CreateVolumeMount(SecureMountPath, EncryptionKeyXMLFileName))

	// Mount a volume /config/configDropins/overrides/encryptionKeyMount.xml to import the Liberty Password Encryption Key
	MountSecretAsVolume(pts, operatorShortName+ManagedEncryptionMountServerXML+passwordEncryptionMetadata.Name, CreateVolumeMount(overridesMountPath, EncryptionKeyMountXMLFileName))
}

// ConfigureLTPA setups the shared-storage for LTPA keys file generation
func ConfigureLTPAConfig(pts *corev1.PodTemplateSpec, la *olv1.OpenLibertyApplication, operatorShortName string, ltpaSecretName string, ltpaSuffixName string) {
	// Mount a volume /output/liberty-operator/ltpa.keys to store the ltpa.keys file
	MountSecretAsVolume(pts, ltpaSecretName, CreateVolumeMount(SecureMountPath, LTPAKeysFileName))

	// Mount a volume /output/liberty-operator/managedLTPA.xml to store the Liberty Server XML
	MountSecretAsVolume(pts, operatorShortName+LTPAServerXMLSuffix+ltpaSuffixName, CreateVolumeMount(SecureMountPath, LTPAKeysXMLFileName))

	// Mount a volume /config/configDropins/overrides/managedLTPAMount.xml to import the managedLTPA.xml file
	MountSecretAsVolume(pts, operatorShortName+LTPAServerXMLMountSuffix+ltpaSuffixName, CreateVolumeMount(overridesMountPath, LTPAKeysMountXMLFileName))
}

func MountSecretAsVolume(pts *corev1.PodTemplateSpec, secretName string, volumeMount corev1.VolumeMount) {
	if !isVolumeMountFound(pts, volumeMount.Name) {
		pts.Spec.Containers[0].VolumeMounts = append(pts.Spec.Containers[0].VolumeMounts, volumeMount)
	}
	if !isVolumeFound(pts, volumeMount.Name) {
		vol := corev1.Volume{
			Name: volumeMount.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
					Items: []corev1.KeyToPath{{
						Key:  volumeMount.SubPath,
						Path: volumeMount.SubPath,
					}},
				},
			},
		}
		pts.Spec.Volumes = append(pts.Spec.Volumes, vol)
	}
}

func CustomizeEncryptionKeyXML(managedEncryptionXMLSecret *corev1.Secret, encryptionKey string) error {
	if managedEncryptionXMLSecret.StringData == nil {
		managedEncryptionXMLSecret.StringData = make(map[string]string)
	}
	serverXML, err := os.ReadFile("internal/controller/assets/encryption.xml")
	if err != nil {
		return err
	}
	severXMLString := strings.Replace(string(serverXML), "WLP_PASSWORD_ENCRYPTION_KEY", encryptionKey, 1)
	managedEncryptionXMLSecret.StringData[EncryptionKeyXMLFileName] = severXMLString
	return nil
}

func CustomizeLTPAServerXML(xmlSecret *corev1.Secret, la *olv1.OpenLibertyApplication, encryptedPassword string) error {
	xmlSecret.StringData = make(map[string]string)
	managedLTPADir := strings.Replace(SecureMountPath, "/output", "${server.output.dir}", 1)
	serverXML, err := os.ReadFile("internal/controller/assets/ltpa.xml")
	if err != nil {
		return err
	}
	severXMLString := strings.Replace(string(serverXML), "LTPA_KEYS_FILE_NAME", managedLTPADir+"/"+LTPAKeysFileName, 1)
	severXMLString = strings.Replace(severXMLString, "LTPA_KEYS_PASSWORD", encryptedPassword, 1)
	xmlSecret.StringData[LTPAKeysXMLFileName] = severXMLString
	return nil
}

func CustomizeLibertyFileMountXML(mountingPasswordKeySecret *corev1.Secret, mountXMLFileName string, fileLocation string) error {
	if mountingPasswordKeySecret.StringData == nil {
		mountingPasswordKeySecret.StringData = make(map[string]string)
	}
	serverXML, err := os.ReadFile("internal/controller/assets/mount.xml")
	if err != nil {
		return err
	}
	severXMLString := strings.Replace(string(serverXML), "MOUNT_LOCATION", fileLocation, 1)
	mountingPasswordKeySecret.StringData[mountXMLFileName] = severXMLString
	return nil
}

// Converts a file name into a lowercase word separated string
// Example: managedLTPASecret.xml -> managed-ltpa-secret-xml
func parseMountName(fileName string) string {
	i := 0
	n := len(fileName)
	mountName := ""
	previousUpper := false
	for i < n {
		ch := string(fileName[i])
		if ch == "." {
			mountName += "-"
		} else if ch == strings.ToUpper(ch) {
			if !previousUpper && i > 0 {
				mountName += "-"
			}
			mountName += strings.ToLower(ch)
			previousUpper = true
		} else {
			mountName += ch
			previousUpper = false
		}
		i += 1
	}
	return mountName
}

func kebabToCamelCase(inputString string) string {
	i := 0
	n := len(inputString)
	outputString := ""
	for i < n && string(inputString[i]) == "-" {
		i += 1
	}
	for i < n {
		ch := string(inputString[i])
		if ch == "-" {
			if i < n-1 {
				outputString += strings.ToUpper(string(inputString[i+1]))
			}
			i += 2
		} else {
			outputString += ch
			i += 1
		}
	}
	return outputString
}

func CreateVolumeMount(mountPath string, fileName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      parseMountName(fileName),
		MountPath: mountPath + "/" + fileName,
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

func IsOperandVersionString(version string) bool {
	if !strings.Contains(version, "_") {
		return false
	}
	if version[0] != 'v' {
		return false
	}
	versionArray := strings.Split(version, "_")
	n := len(versionArray)
	return n == 3
}

func GetFirstNumberFromString(target string) string {
	k := 0
	for k < len(target) {
		if _, err := strconv.Atoi(string(target[k])); err != nil {
			break
		}
		k += 1
	}
	return target[:k]
}

// Converts semantic version string "a.b.c" to format "va_b_c"
func GetOperandVersionString() (string, error) {
	if !strings.Contains(OperandVersion, ".") {
		return "", fmt.Errorf("expected OperandVersion to be in semantic version format")
	}
	versionArray := strings.Split(OperandVersion, ".")
	n := len(versionArray)
	if n != 3 {
		return "", fmt.Errorf("expected OperandVersion to be in semantic version format with 3 arguments")
	}
	finalVersion := "v"
	for i, version := range versionArray {
		if i < n-1 {
			if version != GetFirstNumberFromString(version) {
				return "", fmt.Errorf("expected OperandVersion not to contain build manifest data in the first two arguments")
			}
			finalVersion += version
			finalVersion += "_"
		} else {
			// trim the end for possible build metadata
			finalVersion += GetFirstNumberFromString(version)
		}
	}
	return finalVersion, nil
}

func GetCommaSeparatedArray(stringList string) []string {
	if strings.Contains(stringList, ",") {
		return strings.Split(stringList, ",")
	}
	return []string{stringList}
}

var letterNums = []rune("abcdefghijklmnopqrstuvwxyz1234567890")
var letterNums2 = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")

func GetRandomAlphanumeric(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterNums2[rand.IntN(len(letterNums2))]
	}
	return string(b)
}

func GetRandomLowerAlphanumericSuffix(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterNums[rand.IntN(len(letterNums))]
	}
	return "-" + string(b)
}

func IsLowerAlphanumericSuffix(suffix string) bool {
	for _, ch := range suffix {
		numCheck := int(ch - '0')
		lowerAlphaCheck := int(ch - 'a')
		if !((numCheck >= 0 && numCheck <= 9) || (lowerAlphaCheck >= 0 && lowerAlphaCheck <= 25)) {
			return false
		}
	}
	return true
}

func GetCommaSeparatedString(stringList string, index int) (string, error) {
	if stringList == "" {
		return "", fmt.Errorf("there is no element")
	}
	if strings.Contains(stringList, ",") {
		for i, val := range strings.Split(stringList, ",") {
			if index == i {
				return val, nil
			}
		}
	} else {
		if index == 0 {
			return stringList, nil
		}
		return "", fmt.Errorf("cannot index string list with only one element")
	}
	return "", fmt.Errorf("element not found")
}

// returns the index of the contained value in stringList or else -1
func CommaSeparatedStringContains(stringList string, value string) int {
	if strings.Contains(stringList, ",") {
		for i, label := range strings.Split(stringList, ",") {
			if value == label {
				return i
			}
		}
	} else if stringList == value {
		return 0
	}
	return -1
}

func IsValidOperandVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	if version[0] != 'v' {
		return false
	}
	if !strings.Contains(version[1:], "_") {
		return false
	}
	versions := strings.Split(version[1:], "_")
	if len(versions) != 3 {
		return false
	}
	for _, version := range versions {
		if len(GetFirstNumberFromString(version)) == 0 {
			return false
		}
	}

	return true
}

func CompareOperandVersion(a string, b string) int {
	arrA := strings.Split(a[1:], "_")
	arrB := strings.Split(b[1:], "_")
	for i := range arrA {
		intA, _ := strconv.ParseInt(GetFirstNumberFromString(arrA[i]), 10, 64)
		intB, _ := strconv.ParseInt(GetFirstNumberFromString(arrB[i]), 10, 64)
		if intA != intB {
			return int(intA - intB)
		}
	}
	return 0
}
