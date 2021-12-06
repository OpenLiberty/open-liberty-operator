#!/bin/bash

#########################################################################################
#
#
#           Script to build images for all releases and daily.
#           Note: Assumed to run under <operator root>/scripts
#
#
#########################################################################################

set -Eeo pipefail

readonly usage="Usage: $0 -u <docker-username> -p <docker-password> --image [registry/]<repository>/<image> --target <daily|releases|release-tag>"
readonly script_dir="$(dirname "$0")"
readonly release_blocklist="${script_dir}/release-blocklist.txt"

main() {
  parse_args "$@"

  if [[ -z "${TARGET}" ]]; then
    echo "****** Missing target release for operator build, see usage"
    echo "${usage}"
    exit 1
  fi

  if [[ -z "${IMAGE}" ]]; then
    echo "****** Missing target image for operator build, see usage"
    echo "${usage}"
    exit 1
  fi

  if [[ -z "${DOCKER_USERNAME}" || -z "${DOCKER_PASSWORD}" ]]; then
    echo "****** Missing docker authentication information, see usage"
    echo "${usage}"
    exit 1
  fi

  ## login to docker
  if [[ -z "${REGISTRY}" ]]; then
    echo "${DOCKER_PASSWORD}" | docker login -u "${USER}" --password-stdin
  else
    echo "${DOCKER_PASSWORD}" | docker login "${REGISTRY}" -u "${USER}" --password-stdin
  fi

  # Build target release(s)
  if [[ "${TARGET}" != "releases" ]]; then
    "${script_dir}/build-release.sh" -u "${DOCKER_USERNAME}" -p "${DOCKER_PASSWORD}" --release "${TARGET}" --image "${IMAGE}"
  else
    build_releases
  fi
}

build_releases() {
  tags="$(git tag -l)"
  while read -r tag; do
    if [[ -z "${tag}" ]]; then
      break
    fi

    # Skip any releases listed in the release blocklist
    if grep -q "^${tag}$" "${release_blocklist}"; then
      echo "Release ${tag} found in blocklist. Skipping..."
      continue
    fi

    "${script_dir}/build-release.sh" -u "${DOCKER_USERNAME}" -p "${DOCKER_PASSWORD}" --release "${tag}" --image "${IMAGE}"
  done <<< "${tags}"
}

build_release() {
  local release="$1"
  local full_image="${IMAGE}:${release}-${arch}"
  echo "*** Building ${full_image} for ${arch}"
  docker build -t "${full_image}" .
  if [[ -n "${REGISTRY}" ]]; then
    docker tag "${full_image}" "${REGISTRY}/${full_image}"
  fi
  return $?
}

push_release() {
  local release="$1"

  if [[ "${TRAVIS}" = "true" && "${TRAVIS_PULL_REQUEST}" = "false" && "${TRAVIS_BRANCH}" = "master" ]]; then
    echo "****** Pushing image: ${IMAGE}:${release}-${arch}"
    docker push "${IMAGE}:${release}-${arch}"
  else
    echo "****** Skipping push for branch ${TRAVIS_BRANCH}"
  fi

  if [[ -n "${REGISTRY}" ]]; then
    echo "****** Pushing image to scan: ${IMAGE}:${release}-${arch}"
    docker push "${REGISTRY}/${IMAGE}:${release}-${arch}"
  fi
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
    -u)
      shift
      readonly DOCKER_USERNAME="${1}"
      ;;
    -p)
      shift
      readonly DOCKER_PASSWORD="${1}"
      ;;
    --registry)
      shift
      readonly REGISTRY="${1}"
      ;;
    --image)
      shift
      readonly IMAGE="${1}"
      ;;
    --target)
      shift
      readonly TARGET="${1}"
      ;;
    *)
      echo "Error: Invalid argument - $1"
      echo "$usage"
      exit 1
      ;;
    esac
    shift
  done
}

main "$@"
