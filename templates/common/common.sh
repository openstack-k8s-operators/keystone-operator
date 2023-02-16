#!/bin//bash
#
# Copyright 2022 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

set -e

function merge_config_dir {
    echo merge config dir $1
    for conf in $(find $1 -type f); do
        conf_base=$(basename $conf)

        # If CFG already exist in ../merged and is not a json file,
        # we expect for now it can be merged using crudini.
        # Else, just copy the full file.
        if [[ -f /var/lib/config-data/merged/${conf_base} && ${conf_base} != *.json ]]; then
            echo merging ${conf} into /var/lib/config-data/merged/${conf_base}
            crudini --merge /var/lib/config-data/merged/${conf_base} < ${conf}
        else
            echo copy ${conf} to /var/lib/config-data/merged/
            cp -f ${conf} /var/lib/config-data/merged/
        fi
    done
}
