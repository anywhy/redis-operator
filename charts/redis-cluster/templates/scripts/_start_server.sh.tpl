#!/bin/sh

# This script is used to start pd containers in kubernetes cluster

# Use DownwardAPIVolumeFiles to store informations of the cluster:
# https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/#the-downward-api
#
#   runmode="normal/debug"
#

set -uo pipefail

ANNOTATIONS="/etc/podinfo/annotations"

if [[ ! -f "${ANNOTATIONS}" ]]
then
    echo "${ANNOTATIONS} does't exist, exiting."
    exit 1
fi
source ${ANNOTATIONS} 2>/dev/null

# Default ROLE
ROLE_NAME=${ROLE_NAME:-master}

ARGS=/etc/redis/redis.conf

{{- if (eq .Values.redis.mode "replica") }}
# the general form of variable PEER_MASTER_SERVICE_NAME is: "<clusterName>-master-peer"
master_url="${PEER_MASTER_SERVICE_NAME}.${NAMESPACE}.svc 6379"

elapseTime=0
period=1
threshold=30
while true; do
    sleep ${period}
    elapseTime=$(( elapseTime+period ))

    if [[ ${elapseTime} -ge ${threshold} ]]
    then
		echo "waiting for redis master role server node timeout, start as master node" >&2
		break
	fi

	if [[ -s /etc/podinfo/redisrole && `cat /etc/podinfo/redisrole |wc -l` -eq 0 ]]; then
		ROLE_NAME=$(cat /etc/podinfo/redisrole)
        break
	fi
	
done

if [[ X${ROLE_NAME} != Xmaster ]]; then
	ARGS="${ARGS} --slaveof ${master_url}"
fi
{{- end }}

echo "starting redis-server ..."
echo "redis-server ${ARGS}"
exec redis-server ${ARGS}
