#!/bin/bash

# Handle custom CA if needed
if [[ $USE_CUSTOM_CA == "true" ]]; then
    update-ca-trust
fi

# Set default log flush delay for Vector logging (can be overridden)
export LOG_FLUSH_DELAY_SECONDS=${LOG_FLUSH_DELAY_SECONDS:-2}

# Execute main application
/usr/bin/terraform-repo-executor
