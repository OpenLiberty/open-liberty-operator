package image

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/client/auth"
	"github.com/docker/docker/api/types"
	dockerregistry "github.com/docker/docker/registry"
	"github.com/go-logr/logr"
	godigest "github.com/opencontainers/go-digest"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/credentialprovider"
	"k8s.io/kubernetes/pkg/credentialprovider/secrets"
)

const (
	NilLibertyVersion = "0.0.0.0"
)

var ValidLibertyVersionLabels = []string{"liberty.version", "io.openliberty.version", "com.ibm.websphere.liberty.version", "org.opencontainers.image.version", "version"}

type NamespaceCredentialsContext struct {
	transport         http.RoundTripper
	insecureTransport http.RoundTripper
	secrets           []corev1.Secret
	contexts          sync.Map
	reqLogger         logr.Logger
}

func NewNamespaceCredentialsContext(reqLogger logr.Logger, secrets []corev1.Secret, namespace string) *NamespaceCredentialsContext {
	return &NamespaceCredentialsContext{
		secrets:   secrets,
		reqLogger: reqLogger.WithValues("Request.Namespace", namespace).V(2).WithName("NamespaceCredentialsContext"),
	}
}

func convertImageV1ToReferenceDockerImageReference(refIn imagev1.DockerImageReference) reference.DockerImageReference {
	return reference.DockerImageReference{
		Registry:  refIn.Registry,
		Namespace: refIn.Namespace,
		Name:      refIn.Name,
		Tag:       refIn.Tag,
		ID:        refIn.ID,
	}
}

func (s *NamespaceCredentialsContext) Repository(
	ctx context.Context,
	refIn imagev1.DockerImageReference,
	pullSecret *corev1.Secret,
	insecure bool,
) (distribution.Repository, error) {
	ref := convertImageV1ToReferenceDockerImageReference(refIn)
	defRef := ref.DockerClientDefaults()
	repo := defRef.AsRepository().Exact()
	if ctxIf, ok := s.contexts.Load(repo); ok {
		importCtx := ctxIf.(*registryclient.Context)
		return importCtx.Repository(ctx, defRef.RegistryURL(), defRef.RepositoryName(), insecure)
	}

	instanceKeyring := &credentialprovider.BasicDockerKeyring{}
	if pullSecret != nil {
		if config, err := credentialprovider.ReadDockerConfigFileFromBytes(pullSecret.Data[".dockerconfigjson"]); err != nil {
			s.reqLogger.Info(fmt.Sprintf("Proceeding without instance pull secret credentials; pull secret is missing field .data.dockerconfigjson; %v", err))
		} else {
			instanceKeyring.Add(nil, config)
			s.reqLogger.Info("Added pull secret config to the keyring")
		}
	}

	keyring, err := secrets.MakeDockerKeyring(s.secrets, instanceKeyring)
	if err != nil {
		s.reqLogger.Error(err, "Failed to create docker keyring")
		return nil, err
	}

	var credentials auth.CredentialStore = registryclient.NoCredentials
	if auths, found := keyring.Lookup(defRef.String()); found {
		credentials = dockerregistry.NewStaticCredentialStore(&types.AuthConfig{
			Username: auths[0].Username,
			Password: auths[0].Password,
		})
		s.reqLogger.Info(fmt.Sprintf("Created auth credentials for user %s based on image ref %s", auths[0].Username, defRef.String()))
	}

	importCtx := registryclient.NewContext(s.transport, s.insecureTransport).WithCredentials(credentials)
	s.contexts.Store(repo, importCtx)
	return importCtx.Repository(ctx, defRef.RegistryURL(), defRef.RepositoryName(), insecure)
}

func (s *NamespaceCredentialsContext) GetContainerImageMetadata(ctx context.Context, imageRef imagev1.DockerImageReference, pullSecret *corev1.Secret, insecure bool) (string, *runtime.RawExtension, error) {
	imageMetadata := &runtime.RawExtension{}
	repo, err := s.Repository(ctx, imageRef, pullSecret, insecure)
	if err != nil {
		return "", nil, fmt.Errorf("Failed to create a repository; %v", err)
	}

	service, err := repo.Manifests(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("Failed to get the repository manifest service; %v", err)
	}

	var idOrDigest string
	var manifest distribution.Manifest
	imageRefName := ""
	if imageRef.Tag != "" {
		manifest, err = service.Get(ctx, "", distribution.WithTag(imageRef.Tag))
		if err != nil {
			return "", nil, fmt.Errorf("Could not pull manifest for tag %s; %v", imageRef.Tag, err)
		}
		imageRefName = imageRef.Tag
		idOrDigest = imageRef.Tag
	}

	var manifestDigest godigest.Digest
	if imageRef.ID != "" {
		manifestDigest = godigest.Digest(imageRef.ID)
		manifest, err = service.Get(ctx, manifestDigest)
		if err != nil {
			return "", nil, fmt.Errorf("Could not pull manifest for id %s; %v", imageRef.ID, err)
		}
		imageRefName = imageRef.ID
		idOrDigest = imageRef.ID
	} else {
		// set the sub manifest for image validation
		manifestDigest = manifest.References()[0].Digest

		// if the manifest list digest exists, set this as the .status.imageReference
		_, payload, err := manifest.Payload()
		if err == nil {
			payloadDigest := godigest.FromBytes(payload)
			idOrDigest = string(payloadDigest)
		} else {
			// otherwise, use the sub manifest as the .status.imageReference
			idOrDigest = string(manifestDigest)
		}
	}

	blobStore := repo.Blobs(ctx)
	manifestDigest, err = createImageFromManifestList(ctx, service, blobStore, imageMetadata, manifest, imageRefName)
	if err == nil {
		return idOrDigest, imageMetadata, nil
	}
	err = createSchema2Image(ctx, blobStore, imageMetadata, manifest, manifestDigest, imageRefName)
	if err == nil {
		return idOrDigest, imageMetadata, nil
	}
	err = createOCIImage(ctx, blobStore, imageMetadata, manifest, manifestDigest, imageRefName)
	if err == nil {
		return idOrDigest, imageMetadata, nil
	}
	return "", nil, fmt.Errorf("Could not deserialize manifest for ref %s; %v", imageRefName, err)
}

func createImageFromManifestList(ctx context.Context, service distribution.ManifestService, blobStore distribution.BlobStore, imageMetadata *runtime.RawExtension, manifest distribution.Manifest, imageRefName string) (godigest.Digest, error) {
	var schema2Err, ociErr error
	if deserializedManifestList, found := manifest.(*manifestlist.DeserializedManifestList); found {
		manifests := deserializedManifestList.ManifestList.Manifests
		if len(manifests) == 0 {
			return "", fmt.Errorf("Failed to parse manifest list; the list is empty")
		}
		var manifestDigest godigest.Digest
		if manifestDigest == "" {
			for _, manifestDescriptor := range manifests {
				if manifestDescriptor.Platform.Architecture == "amd64" && manifestDescriptor.Platform.OS == "linux" {
					manifestDigest = manifestDescriptor.Digest
					break
				}
			}
		}
		subManifest, err := service.Get(ctx, manifestDigest)
		if err != nil {
			return "", fmt.Errorf("Failed to get manifest for id %s; %v", manifestDigest, err)
		}
		schema2Err = createSchema2Image(ctx, blobStore, imageMetadata, subManifest, manifestDigest, imageRefName)
		if schema2Err == nil {
			return manifestDigest, nil
		}

		ociErr = createOCIImage(ctx, blobStore, imageMetadata, subManifest, manifestDigest, imageRefName)
		if ociErr == nil {
			return manifestDigest, nil
		}
	}
	errStrings := []string{}
	if schema2Err != nil {
		errStrings = append(errStrings, fmt.Sprintf("%v", schema2Err))
	}
	if ociErr != nil {
		errStrings = append(errStrings, fmt.Sprintf("%v", ociErr))
	}
	return "", fmt.Errorf("Failed to parse schema2 manifest list; %s", strings.Join(errStrings, "; "))
}

func createSchema2Image(ctx context.Context, blobStore distribution.BlobStore, imageMetadata *runtime.RawExtension, manifest distribution.Manifest, manifestDigest godigest.Digest, imageRefName string) error {
	if deserializedManifest, found := manifest.(*schema2.DeserializedManifest); found {
		blob, err := blobStore.Get(ctx, deserializedManifest.Config.Digest)
		if err != nil {
			return fmt.Errorf("Failed to get schema2 digest from blob for ref %s; %v", imageRefName, err)
		}
		if err := createUnstructuredContainerImage(deserializedManifest, manifestDigest, imageMetadata, blob); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("Failed to parse schema2 manifest")
}

func createOCIImage(ctx context.Context, blobStore distribution.BlobStore, imageMetadata *runtime.RawExtension, manifest distribution.Manifest, manifestDigest godigest.Digest, imageRefName string) error {
	if deserializedManifest, found := manifest.(*ocischema.DeserializedManifest); found {
		blob, err := blobStore.Get(ctx, deserializedManifest.Config.Digest)
		if err != nil {
			return fmt.Errorf("Failed to get ocischema digest from blob for ref %s; %v", imageRefName, err)
		}
		if err := createUnstructuredContainerImage(deserializedManifest, manifestDigest, imageMetadata, blob); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("Failed to parse OCI manifest")
}

func createUnstructuredContainerImage(manifest distribution.Manifest, digest godigest.Digest, metadata *runtime.RawExtension, blob []byte) error {
	if err := validateDigest(manifest, digest); err != nil {
		return err
	}
	digestRefName := digest.String()
	containerImage := &unstructured.Unstructured{}
	containerImage.SetKind("ContainerImage")
	containerImage.SetAPIVersion("image.openshift.io/1.0")
	blobMap := map[string]interface{}{}
	if err := json.Unmarshal(blob, &blobMap); err != nil {
		return fmt.Errorf("Failed to unmarshal container image metadata blobMap for ref %s; %v", digestRefName, err)
	}

	if err := unstructured.SetNestedField(containerImage.Object, blobMap["config"], "Config"); err != nil {
		return fmt.Errorf("Failed to marshal container image metadata blobMap config for ref %s; %v", digestRefName, err)
	}
	rawBytes, err := json.Marshal(containerImage.Object)
	if err != nil {
		return fmt.Errorf("Failed to marshal container image metadata objectMap for ref %s; %v", digestRefName, err)
	}
	metadata.Raw = rawBytes
	return nil
}

func validateDigest(manifest distribution.Manifest, digest godigest.Digest) error {
	mediaType, payload, err := manifest.Payload()
	if err != nil {
		return err
	}
	payloadDigest := godigest.FromBytes(payload)
	if len(digest) > 0 && payloadDigest != digest {
		return fmt.Errorf("Failed to validate integrity of the digest; using media type %s the expected digest %s does not match digest parsed %s", mediaType, digest, payloadDigest)
	}
	return nil
}

func ParseLibertyVersionFromContainerImageMetadata(imageMetadata *runtime.RawExtension) string {
	if imageMetadata == nil {
		return ""
	}
	unstructuredImageMeta := &unstructured.Unstructured{}
	if err := json.Unmarshal(imageMetadata.Raw, unstructuredImageMeta); err != nil {
		return ""
	}
	if imageLabels, found, err := unstructured.NestedFieldNoCopy(unstructuredImageMeta.Object, "Config", "Labels"); err == nil && found {
		imageLabelsMap, isMap := imageLabels.(map[string]interface{})
		if !isMap {
			return ""
		}
		for _, validLibertyVersionLabel := range ValidLibertyVersionLabels {
			if version, versionFound := imageLabelsMap[validLibertyVersionLabel]; versionFound && IsValidLibertyVersion(version.(string)) {
				return version.(string)
			}
		}
	}
	// No version found
	return ""
}

// Returns true if version is a valid Liberty version string and false otherwise
func IsValidLibertyVersion(version string) bool {
	args := strings.Split(version, ".")
	if len(args) != 4 {
		return false
	}

	// year should be a number
	_, err := strconv.Atoi(args[0])
	if err != nil {
		return false
	}

	// 2nd and 3rd args should be "0"
	if args[1] != "0" || args[2] != "0" {
		return false
	}

	// month should be a number
	_, err = strconv.Atoi(args[3])
	if err != nil {
		return false
	}

	return true
}
