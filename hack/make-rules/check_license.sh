# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

result=$(
    find . -type f \( -name "*.go" -o -name "*.sh" \) ! -path '*/vendor/*' ! -path '*/test/*' -exec \
         sh -c 'head -n5 $1 | grep -Eq "(Apache License)" || echo -e  $1' {} {} \;
)

if [ -n "${result}" ]; then
        echo -e "license header checking failed:\\n${result}"
        exit 255
fi