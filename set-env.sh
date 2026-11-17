#!/bin/bash
# =============================================================================
#  set-env.sh — Variables de entorno para actuar como Org1 o Org2
#  Uso:  source ./set-env.sh org1     (Laboratorio fabricante)
#        source ./set-env.sh org2     (Distribuidor mayorista)
# =============================================================================
FABRIC_DIR="${FABRIC_DIR:-$HOME/blockchain-lab/fabric-samples}"
NETWORK_DIR="$FABRIC_DIR/test-network"

export PATH="$FABRIC_DIR/bin:$PATH"
export FABRIC_CFG_PATH="$FABRIC_DIR/config/"
export CORE_PEER_TLS_ENABLED=true
export ORDERER_CA="$NETWORK_DIR/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem"
export PEER1_TLS="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem"
export PEER2_TLS="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/tlsca/tlsca.org2.example.com-cert.pem"

case "${1:-org1}" in
  org1|1)
    export CORE_PEER_LOCALMSPID="Org1MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE="$PEER1_TLS"
    export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
    export CORE_PEER_ADDRESS="localhost:7051"
    ;;
  org2|2)
    export CORE_PEER_LOCALMSPID="Org2MSP"
    export CORE_PEER_TLS_ROOTCERT_FILE="$PEER2_TLS"
    export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
    export CORE_PEER_ADDRESS="localhost:9051"
    ;;
  *)
    echo "Uso: source ./set-env.sh org1|org2" >&2
    return 1 2>/dev/null || exit 1
    ;;
esac

echo "MSP activo:  $CORE_PEER_LOCALMSPID"
echo "Peer activo: $CORE_PEER_ADDRESS"
