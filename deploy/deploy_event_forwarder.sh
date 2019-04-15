#! /bin/bash -eo pipefail

function usage {
    echo
    echo "Usage:"
    echo ""
    echo "  <customer> = customer name"
    echo "  <size> = size of forwarder (S, M, L, XL, XLM)"        
    echo "  <cb-hostname> = Hostname of Carbon Black server"
    echo "  <rabbit-username> = RabbitMQ username"
    echo "  <rabbit-password> = RabbitMQ password"
    echo "  <rabbit-ssl> = true/false whether rabbit should connect over tls"
    echo ""
    echo "  e.g deploy_event_forwarder green rc XL cb password false"
}

if [ "$#" -ne "6" ]; then
   usage
   exit
fi

CUSTOMER_NAME="$1"
EVENT_FORWARDER_SIZE="$2"
CB_SERVER_HOSTNAME="$3"
CB_RABBIT_USERNAME="$4"
CB_RABBIT_PASSWORD="$5"
CB_RABBIT_SSL="$6"

echo "${CUSTOMER_NAME}"

if [ "${EVENT_FORWARDER_SIZE}" == "S" ]; then
    CPU_LIMIT=2
    CPU_REQUEST=1
    MEM_LIMIT=4096Mi
    MEM_REQUEST=4096Mi
    REPLICAS=1
elif [ "${EVENT_FORWARDER_SIZE}" == "M" ]; then
    CPU_LIMIT=4
    CPU_REQUEST=2
    MEM_LIMIT=8192Mi
    MEM_REQUEST=8192Mi
    REPLICAS=1
elif [ "${EVENT_FORWARDER_SIZE}" == "L" ]; then
    CPU_LIMIT=8
    CPU_REQUEST=4
    MEM_LIMIT=16384Mi
    MEM_REQUEST=16384Mi
    REPLICAS=1
elif [ "${EVENT_FORWARDER_SIZE}" == "XL" ]; then
    CPU_LIMIT=16
    CPU_REQUEST=4
    MEM_LIMIT=16384Mi
    MEM_REQUEST=16384Mi
    REPLICAS=1
elif [ "${EVENT_FORWARDER_SIZE}" == "XLM" ]; then
    CPU_LIMIT=16
    CPU_REQUEST=4
    MEM_LIMIT=20480Mi
    MEM_REQUEST=20480Mi
    REPLICAS=1
 else
    printf "Invalid event_forwarder size [%s] specified for %s \n" "${EVENT_FORWARDER_SIZE}" "${CUSTOMER_NAME}"
    exit
fi

if [ "${CB_RABBIT_SSL}" == "true" ]; then
    RABBIT_PORT=5671
else
    RABBIT_PORT=5004
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Deploy the secret
TMPFILE="$(mktemp /tmp/${CUSTOMER_NAME}_event_forwarder.conf.XXXXXXXXXXXXXX)"
cp ${DIR}/event-forwarder.conf.template "${TMPFILE}"

sed -i -e "s,@@CUSTOMER_NAME@@,${CUSTOMER_NAME}," "${TMPFILE}"
sed -i -e "s,@@CB_SERVER_HOSTNAME@@,${CB_SERVER_HOSTNAME}," "${TMPFILE}"
sed -i -e "s,@@CB_RABBIT_USERNAME@@,${CB_RABBIT_USERNAME}," "${TMPFILE}"
sed -i -e "s,@@CB_RABBIT_PASSWORD@@,${CB_RABBIT_PASSWORD}," "${TMPFILE}"
sed -i -e "s,@@CB_RABBIT_PORT@@,${RABBIT_PORT}," "${TMPFILE}"
sed -i -e "s,@@CB_RABBIT_QUEUE_NAME@@,redcanary-s3," "${TMPFILE}"
sed -i -e "s,@@DESTINATION_S3_REGION@@,us-east-1," "${TMPFILE}"
sed -i -e "s,@@DESTINATION_S3_BUCKET@@,rc-native," "${TMPFILE}"
sed -i -e "s,@@CB_RABBIT_SSL@@,${CB_RABBIT_SSL}," "${TMPFILE}"

cmd="kubectl delete secret "${CUSTOMER_NAME}-event-forwarder-config""
echo $cmd
$cmd || true

cmd="kubectl create secret generic "${CUSTOMER_NAME}-event-forwarder-config" --from-file=cb-event-forwarder-s3.conf=${TMPFILE}"
echo $cmd
$cmd

# Deploy the deployment
TMPFILE="$(mktemp /tmp/${CUSTOMER_NAME}_event_forwarder.yaml.XXXXXXXXXXXXXX)"
cp ${DIR}/event-forwarder.yaml.template "${TMPFILE}"

sed -i -e "s,@@CUSTOMER_NAME@@,${CUSTOMER_NAME}," "${TMPFILE}"
sed -i -e "s,@@REPLICAS@@,${REPLICAS}," "${TMPFILE}"
sed -i -e "s,@@CPU_LIMIT@@,${CPU_LIMIT}," "${TMPFILE}"
sed -i -e "s,@@CPU_REQUEST@@,${CPU_REQUEST}," "${TMPFILE}"
sed -i -e "s,@@MEM_LIMIT@@,${MEM_LIMIT}," "${TMPFILE}"
sed -i -e "s,@@MEM_REQUEST@@,${MEM_REQUEST}," "${TMPFILE}"

cmd="kubectl apply -f ${TMPFILE}"
echo $cmd
$cmd

# Deploy an hpa for the deployment
cmd="kubectl autoscale deployment "${CUSTOMER_NAME}-event-forwarder" --cpu-percent=80 --min=1 --max=10"
echo $cmd
$cmd
