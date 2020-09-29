#!/bin/bash

usage="usage: $(basename "$0") [-d] [-h] [-v]

This script will build the phenix binary using a temporary Docker image to
avoid having to install build dependencies locally.

Note that the '-d' flag only disables authentication in the client-side UI
code when building it. To fully disable authentication, the 'phenix ui'
command must not be passed the '--jwt-signing-key' option at runtime.

If not provided, the '-v' flag will default to the hash of the current git
repository commit.

where:
    -d      disable phenix web UI authentication
    -h      show this help text
    -v      version number to use for phenix"


auth=enabled
version=$(git log -1 --format="%h")
commit=$(git log -1 --format="%h")


# loop through positional options/arguments
while getopts ':dhv:' option; do
    case "$option" in
        d)  auth=disabled          ;;
        h)  echo -e "$usage"; exit ;;
        v)  version="$OPTARG"      ;;
        \?) echo -e "illegale option: -$OPTARG\n" >$2
            echo -e "$usage" >&2
            exit 1 ;;
    esac
done


echo    "phenix web UI authentication: $auth"
echo    "phenix version number:        $version"
echo -e "phenix commit:                $commit\n"


which docker &> /dev/null

if (( $? )); then
  echo "Docker must be installed (and in your PATH) to use this build script. Exiting."
  exit 1
fi


echo "Building temporary Docker image 'phenix:build' (this might take a while)..."

output=$(
  docker build -t phenix:build          \
    --build-arg PHENIX_WEB_AUTH=$auth   \
    --build-arg PHENIX_VERSION=$version \
    --build-arg PHENIX_COMMIT=$commit   \
    -f Dockerfile . 2>&1
)

if (( $? )); then
  echo -e "\nERROR: $output\n"
  echo    "Could not build temporary 'phenix:build' Docker image (see above). Exiting."
  exit 1
fi

echo -e "Docker image 'phenix:build' built successfully.\n"


echo "Extracting phenix binary from Docker image..."

mkdir -p bin

container=$(docker create phenix:build 2>&1)

if (( $? )); then
  echo -e "\nERROR: $container\n"
  echo    "Could not extract phenix binary from Docker image (see above). Exiting."
  exit 1
fi

output=$(docker cp $container:/usr/local/bin/phenix bin/phenix 2>&1)

if (( $? )); then
  echo -e "\nERROR: $output\n"
  echo    "Could not extract phenix binary from Docker image (see above). Exiting."
  exit 1
fi

output=$(docker rm -f $container 2>&1)

if (( $? )); then
  echo -e "\nERROR: $output\n"
  echo    "Could not remove temporary Docker container (see above)."
fi

echo -e "Extracted phenix binary from Docker image successfully (see 'bin/phenix').\n"


echo "Deleting temporary Docker image 'phenix:build'..."

output=$(docker rmi phenix:build 2>&1)

if (( $? )); then
  echo -e "\nERROR: $output\n"
  echo    "Could not delete temporary 'phenix:build' Docker image (see above). Exiting."
  exit 1
fi

echo "Docker image 'phenix:build' deleted successfully."