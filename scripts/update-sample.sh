#!/bin/sh

# This script fetches the sha256 for the latest version of the open liberty getting started container
# image, and then edits the sample deployment and the csv for the operator, and inserts the tag

if ! skopeo -v ; then
  echo "Skopeo is not installed. Sample sha will not be updated"
  exit
fi

echo "Editing sample tag"
SHA=$(skopeo inspect docker://icr.io/appcafe/open-liberty/samples/getting-started:latest | jq '.Digest'| sed -e 's/"//g')
if [ -z $SHA ]
then
  echo "Couldn't find latest SHA for sample image"
  exit
fi

echo "sha is $SHA"
sed -i.bak "s,getting-started@sha256:[a-zA-Z0-9]*,getting-started@$SHA," config/samples/liberty.websphere.ibm.com_v1_webspherelibertyapplications.yaml
sed -i.bak "s,getting-started@sha256:[a-zA-Z0-9]*,getting-started@$SHA," config/manager/manager.yaml
sed -i.bak "s,getting-started@sha256:[a-zA-Z0-9]*,getting-started@$SHA," internal/deploy/kustomize/daily/base/websphere-liberty-deployment.yaml
# Sed on mac doesn't allow edit in place without making a backup, so need to delete afterwards
rm config/samples/liberty.websphere.ibm.com_v1_webspherelibertyapplications.yaml.bak
rm config/manager/manager.yaml.bak
rm internal/deploy/kustomize/daily/base/websphere-liberty-deployment.yaml.bak

