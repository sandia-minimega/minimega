#!/bin/bash

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

echo "BUILD DOCUMENTATION"

set -e

mkdir -p $ROOT_DIR/doc/content/reference

# build api documentation
echo "apigen"
$ROOT_DIR/bin/apigen -bin $ROOT_DIR/bin/minimega \
            -template $ROOT_DIR/doc/content_templates/minimega_api.template \
            -sections .,mesh,vm,host \
            > $ROOT_DIR/doc/content/reference/minimega.md

$ROOT_DIR/bin/apigen -bin $ROOT_DIR/bin/minirouter \
            -template $ROOT_DIR/doc/content_templates/minirouter_api.template \
            -sections . \
            > $ROOT_DIR/doc/content/reference/minirouter.md

$ROOT_DIR/bin/pyapigen -out $ROOT_DIR/lib/minimega.py $ROOT_DIR/bin/minimega
