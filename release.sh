#!/bin/bash

# Tools needed to run this
# yq - YAML CLI
# gh - GitHub CLI

# Prepare helm chart
VERSION=$1

yq -i ".version = \"${VERSION}\"" charts/cluster-secret/Chart.yaml
yq -i ".appVersion = \"${VERSION}\"" charts/cluster-secret/Chart.yaml
make helm
yq -i ".controllerManager.manager.image.tag = \"v${VERSION}\"" charts/cluster-secret/values.yaml

# Build docker image
make docker-buildx IMG="public.ecr.aws/chistadata/cluster-secret-operator:v${VERSION}"

# Package the helm chart
helm package charts/cluster-secret
helm repo index . --url "https://github.com/ChistaDATA/cluster-secret-operator/releases/download/v${VERSION}/" --merge docs/index.yaml

cp -f index.yaml docs/index.yaml
rm -f index.yaml

# Clean up bad values
sed -i '' '/creationTimestamp/d' config/rbac/role.yaml
sed -i '' '/creationTimestamp/d' config/crd/bases/chistadata.io_clustersecrets.yaml

# Push release changes to the repo
git add charts/cluster-secret/Chart.yaml
git add charts/cluster-secret/values.yaml
git add docs/index.yaml

git commit -m "Release v${VERSION}"
git push origin main

# Create GitHub release
git tag "v${VERSION}"
git push origin "v${VERSION}"
gh release create --generate-notes --verify-tag -R ChistaDATA/cluster-secret-operator "v${VERSION}" "cluster-secret-${VERSION}.tgz"

rm -f "cluster-secret-${VERSION}.tgz"
