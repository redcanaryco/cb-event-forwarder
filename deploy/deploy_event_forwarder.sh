#! /bin/bash
set -eo pipefail

function usage {
    echo
    echo "This script needs $MINPARAMS command-line arguments!"
    echo "Usage:"
    echo ""
    echo "  <source context> = k8 context to operator on"
    echo "  <customer> = k8 context to operator on"
    echo "  <size> = k8 context to operator on"        
    echo ""
    echo "   e.g bin/deploy_event_forwarder.sh green rc XL"
}

function check_usage() {

    K8_CONFIG_PATH="$1"
    if [ ! -f ${K8_CONFIG_PATH}/cb-event-forwarder/event-forwarder.yaml.template ]; then
        echo "cb-event-forwarder/event-forwarder.yaml.template not found - please run from the /deploy directory"
        exit
    fi
}

#
# detokenize -
#    - copies the template
#    - new token values based on parameters into script and values
#
function detokenize_and_apply() {
  #
  #   replace tokens with configuration values
  #
    NAME=$1
    CPU_LIMIT=$2
    CPU_REQUEST=$3
    MEM_LIMIT=$4
    MEM_REQUEST=$5
    REPLICAS=$6
    COMMAND=$7
    ARGS=$8

    TMPFILE="$(mktemp /tmp/${NAME}_event_forwarder.yaml.XXXXXXXXXXXXXX)"
    echo Tempfile used to configure the K8 deployment $TMPFILE

    cp ${K8_CONFIG_PATH}/cb-event-forwarder/event-forwarder.yaml.template "${TMPFILE}"
    sed -i -e "s,@@NAME@@,${NAME}," "${TMPFILE}"
    sed -i -e "s,@@REPLICAS@@,${REPLICAS}," "${TMPFILE}"
    sed -i -e "s,@@COMMAND@@,${COMMAND}," "${TMPFILE}"
    sed -i -e "s,@@ARGS@@,${ARGS}," "${TMPFILE}"
    sed -i -e "s,@@CPU_LIMIT@@,${CPU_LIMIT}," "${TMPFILE}"
    sed -i -e "s,@@CPU_REQUEST@@,${CPU_REQUEST}," "${TMPFILE}"
    sed -i -e "s,@@MEM_LIMIT@@,${MEM_LIMIT}," "${TMPFILE}"
    sed -i -e "s,@@MEM_REQUEST@@,${MEM_REQUEST}," "${TMPFILE}"

    echo "calling --context ${CONTEXT}  kubectl create -f ${TMPFILE}"
    status="$(kubectl --context ${CONTEXT} create -f $TMPFILE)"
#    rm "$TMPFILE"
}

function main() {
    #
    #  This should be replace with call to portal or by the future rabbit-mq autoscaler
    #

    MINPARAMS=3

    if [ "$#" -lt "$MINPARAMS" ]; then
       usage
       exit
    fi

    CONTEXT="${1}.redcanary.io"
    EVENT_FORWARDER_CUSTOMER="$2"
    EVENT_FORWARDER_SIZE="$3"

    K8_CONFIG_PATH="$(pwd)"
    COMMAND="/cb-event-forwarder"

    check_usage "$K8_CONFIG_PATH"

    echo "${EVENT_FORWARDER_CUSTOMER}"

    if [ "${EVENT_FORWARDER_SIZE}" == "S" ]; then
        CPU_LIMIT=2
        CPU_REQUEST=2
        MEM_LIMIT=4096Mi
        MEM_REQUEST=4096Mi
        REPLICAS=1
    elif [ "${EVENT_FORWARDER_SIZE}" == "M" ]; then
        CPU_LIMIT=4
        CPU_REQUEST=4
        MEM_LIMIT=8192Mi
        MEM_REQUEST=8192Mi
        REPLICAS=1
    elif [ "${EVENT_FORWARDER_SIZE}" == "L" ]; then
        CPU_LIMIT=8
        CPU_REQUEST=8
        MEM_LIMIT=16384Mi
        MEM_REQUEST=16384Mi
        REPLICAS=1
    elif [ "${EVENT_FORWARDER_SIZE}" == "XL" ]; then
        CPU_LIMIT=16
        CPU_REQUEST=16
        MEM_LIMIT=16384Mi
        MEM_REQUEST=16384Mi
        REPLICAS=1
    elif [ "${EVENT_FORWARDER_SIZE}" == "XLM" ]; then
        CPU_LIMIT=16
        CPU_REQUEST=16
        MEM_LIMIT=20480Mi
        MEM_REQUEST=20480Mi
        REPLICAS=1
     else
        printf "Invalid event_forwarder size [%s] specified for %s \n" "${EVENT_FORWARDER_SIZE}" "${EVENT_FORWARDER_CUSTOMER}"
        exit
    fi

    ARGS="/etc/cb/${EVENT_FORWARDER_CUSTOMER}-cb-event-forwarder-s3.conf"
    CUST_NAME="${EVENT_FORWARDER_CUSTOMER}-event-forwarder"
    detokenize_and_apply "${CUST_NAME}" $CPU_LIMIT $CPU_REQUEST $MEM_LIMIT $MEM_REQUEST $REPLICAS $COMMAND $ARGS

    #
    #  Deploy an hpa for the deployment
    #
    cmd="kubectl --context ${CONTEXT} autoscale deployment ${CUST_NAME} --cpu-percent=80 --min=1 --max=10"
    echo $cmd
    $cmd

}

main "$@"

# debug
#cat ${TMPFILE}

