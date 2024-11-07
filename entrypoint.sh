#!/bin/bash
if [[ $USE_CUSTOM_CA == "true" ]]; then
    update-ca-trust
fi

/usr/bin/terraform-repo-executor