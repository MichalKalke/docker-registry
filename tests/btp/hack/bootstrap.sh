#!/bin/bash
set -e

if [ -z "$1" ]
  then
    echo "subaccount name not provided"
	exit 1
fi

#mkdir -p tmp

##ensure kyma CLI into /bin folder
#if [ ! -f ../bin/kyma ]; then
#    echo "Kyma binary not found!"
#    mkdir -p ../bin
#    curl -s -L "https://github.com/kyma-project/cli/releases/download/v0.0.0-dev/kyma_$(uname -s)_$(uname -m).tar.gz" | tar -zxvf - -C ../bin kyma
#    echo "Kyma binary downloaded into /bin/kyma"
#fi
#
#export TF_VAR_BTP_SUBACCOUNT=$1
#
## Create a new subaccount with Kyma instance and OIDC
#tofu -chdir=../tf init
#tofu -chdir=../tf apply -auto-approve

##### ---------------------------------------------------------------------------------


### TODO: refactor getting access to the cluster : use btp CLI or kyma CLI ---

##  `kyma alpha get access btp --btpAccessToken --kymaToken --output`  https://github.com/kyma-project/cli/issues/2198

#Generate bot user based access
make headless-kubeconfig

#Generate acces based on service account bound to a selected cluster-role (for the automation purpose) using the one-off bot user based access
CLUSTERROLE=cluster-admin make service-account-kubeconfig

### ---------------------------------------------------------------------------

# add bindings to statefull service instances provisioned in different subaccount (btp object store)
# TF_VAR_BTP_BOT_USER must be assigned to the `Subaccount_viewer` role collection in the provider subaccount (TF_VAR_BTP_PROVIDER_SUBACCOUNT_ID)
KUBECONFIG=tmp/sa-kubeconfig.yaml BTP_PROVIDER_SUBACCOUNT_ID=$TF_VAR_BTP_PROVIDER_SUBACCOUNT_ID make share-sm-service-operator-access
KUBECONFIG=tmp/sa-kubeconfig.yaml make create-object-store-reference

### ---------------------------------------------------------------------------

# TODO: change to enable from experimental channel via kyma v3 cli
KUBECONFIG=tmp/sa-kubeconfig.yaml make enable_docker_registry


# TODO: the following is sort of "kyma push app" equivalent for "cf push"
KUBECONFIG=tmp/sa-kubeconfig.yaml make docker_registry_login
KUBECONFIG=tmp/sa-kubeconfig.yaml make docker_push_simple_app
KUBECONFIG=tmp/sa-kubeconfig.yaml make deploy_simple_app


# CLEANUP
tofu -chdir=../tf destroy -auto-approve
rm -rf tmp
