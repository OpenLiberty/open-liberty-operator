package utils

import (
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// Utility methods specific to Liberty and it's configuration

// CustomizeLibertyEnv adds configured env variables appending configured liberty settings
func CustomizeLibertyEnv(pts *corev1.PodTemplateSpec, la *openlibertyv1beta1.LibertyApplication) {
	// update or set to default depending on CRD config
	if la.Spec.Logs != nil && la.Spec.Logs.ConsoleFormat != nil {
		logVar := corev1.EnvVar{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: *la.Spec.Logs.ConsoleFormat}
		if env, ok := findEnvVar(logVar.Name, pts.Spec.Containers[0].Env); ok {
			// in this case the defined val for consoleFormat is higher priority
			env.Value = *la.Spec.Logs.ConsoleFormat
		} else {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, logVar)
		}
	} else {
		// if undefined set to default, otherwise leave as user defined env
		logVar := corev1.EnvVar{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "json"}
		if _, ok := findEnvVar(logVar.Name, pts.Spec.Containers[0].Env); !ok {
			pts.Spec.Containers[0].Env = append(pts.Spec.Containers[0].Env, logVar)
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
