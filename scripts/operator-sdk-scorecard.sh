#!/bin/bash
mkdir deploy
mkdir deploy/crds
# combine operator_rbac.yaml and operator_deployment.yaml into a single yaml file
echo "---" > deploy/operator_rbac2.yaml.temp
cat ../deploy/operator_rbac.yaml deploy/operator_rbac2.yaml.temp > deploy/operator_rbac2.yaml
cat ../deploy/operator_deployment.yaml >> deploy/operator_rbac2.yaml
operator-sdk scorecard --config operator-sdk-scorecard-config.yaml
rm -r deploy
