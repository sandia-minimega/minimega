#!/bin/bash

# Load configuration values from /etc/default/minimega.
#   The order of precedence is:
#     1. Existing environment variables
#     2. Variables in /etc/default/minimega
#     3. A set of defaults in the calling script
#
# This file is sourced by start-minimega.sh and start-miniweb.sh. It is not
# meant to be executed directly.

if [[ -f "/etc/default/minimega" ]]; then
  # Check if any variables are already set
  while IFS='=' read -r key value; do
    # Skip empty lines and comments
    if [[ -n "$key" && -n "$value" && "$key" != \#* ]]; then
      # Remove surrounding quotes
      value="${value%\"}"
      value="${value#\"}"

      # Only set the variable if it is not already set
      if [[ -z "${!key}" ]]; then
        export "${key}=${value}"
      fi
    fi
  done < <(grep -v '^#' "/etc/default/minimega")
fi
