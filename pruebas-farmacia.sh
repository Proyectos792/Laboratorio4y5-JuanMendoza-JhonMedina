#!/bin/bash
# =============================================================================
#  pruebas-farmacia.sh
#  Pruebas en vivo de las Partes 2 y 3 del Capstone (contexto farmacéutico)
#    A. ActualizarCantidad sobre MED-001 (pasos 1-4 de la guía)
#    B. Validaciones: Org2 no propietaria (Pregunta 9), cantidad <= 0,
#       entregar un lote que no está RECIBIDO (Pregunta 10)
#    C. Cadena de frío rota: el lote queda en cuarentena y no llega a la farmacia
#    D. AgregarDocumento (Pregunta 12) y ConsultarPorVencer
#    E. Altura del ledger (Pregunta 14)
#  Uso:  ./pruebas-farmacia.sh            (con pausas para tomar capturas)
#        AUTO_CONTINUE=true ./pruebas-farmacia.sh
# =============================================================================

FABRIC_DIR="${FABRIC_DIR:-$HOME/blockchain-lab/fabric-samples}"
NETWORK_DIR="$FABRIC_DIR/test-network"
CHANNEL="supplychannel"
CC="supplycc"
AUTO_CONTINUE="${AUTO_CONTINUE:-false}"
PAREJA="${PAREJA:-Juan Mendoza y Jhon Medina}"
VENCE="$(date -v+12m +%Y-%m-%d 2>/dev/null || date -d '+12 months' +%Y-%m-%d)"
SUFIJO="$(date +%H%M%S)"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'
BOLD='\033[1m'; NC='\033[0m'; RED='\033[0;31m'

export PATH="$FABRIC_DIR/bin:$PATH"
export FABRIC_CFG_PATH="$FABRIC_DIR/config/"
export CORE_PEER_TLS_ENABLED=true
export ORDERER_CA="$NETWORK_DIR/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem"
export PEER1_TLS="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem"
export PEER2_TLS="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/tlsca/tlsca.org2.example.com-cert.pem"

[ -x "$FABRIC_DIR/bin/peer" ] || { echo "ERROR: no se encontró $FABRIC_DIR/bin/peer" >&2; exit 1; }

org1() {
  export CORE_PEER_LOCALMSPID="Org1MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE="$PEER1_TLS"
  export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
  export CORE_PEER_ADDRESS="localhost:7051"
  echo -e "  ${CYAN}[contexto] MSP activo: $CORE_PEER_LOCALMSPID · peer $CORE_PEER_ADDRESS${NC}"
}
org2() {
  export CORE_PEER_LOCALMSPID="Org2MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE="$PEER2_TLS"
  export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
  export CORE_PEER_ADDRESS="localhost:9051"
  echo -e "  ${CYAN}[contexto] MSP activo: $CORE_PEER_LOCALMSPID · peer $CORE_PEER_ADDRESS${NC}"
}

# Muestra el comando, lo ejecuta y deja ver el resultado (éxito o error)
invoke() {
  echo -e "  ${BOLD}\$ peer chaincode invoke ... -c '$1'${NC}"
  local out
  if out=$(peer chaincode invoke -o localhost:7050 \
      --ordererTLSHostnameOverride orderer.example.com \
      --tls --cafile "$ORDERER_CA" \
      -C "$CHANNEL" -n "$CC" \
      --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER1_TLS" \
      --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER2_TLS" \
      --waitForEvent -c "$1" 2>&1); then
    echo "$out" | grep -E "txid|invoke successful" | sed 's/^/  /'
    return 0
  else
    echo -e "${RED}$(echo "$out" | grep -E "Error|error" | tail -2 | sed 's/^/  /')${NC}"
    return 1
  fi
}

# Para pruebas negativas: se ESPERA que falle
debe_fallar() {
  if invoke "$1"; then
    echo -e "  ${RED}✗ La transacción NO debía aceptarse${NC}"
  else
    echo -e "  ${GREEN}✓ Rechazada por el chaincode, como se esperaba${NC}"
  fi
}

query() {
  echo -e "  ${BOLD}\$ peer chaincode query -C $CHANNEL -n $CC -c '$1'${NC}"
  peer chaincode query -C "$CHANNEL" -n "$CC" -c "$1" 2>&1 | python3 -m json.tool 2>/dev/null \
    || peer chaincode query -C "$CHANNEL" -n "$CC" -c "$1" 2>&1
}

banner() { echo -e "\n${BOLD}${CYAN}▶ $1${NC}\n"; }
captura() { echo -e "\n  ${YELLOW}📸 CAPTURA: $1${NC}"; }
pause()  {
  [ "$AUTO_CONTINUE" = "true" ] && return 0
  echo -e "\n${YELLOW}  Presiona ENTER para continuar...${NC}"
  read -r
}

clear 2>/dev/null || true
echo "Capstone - $PAREJA - $(date)"

# ═════════════════════════════════════════════════════════════════════════════
banner "A.1 — Cantidad actual de MED-001 (Insulina glargina)"
org1
query '{"function":"LeerProducto","Args":["MED-001"]}'
pause

banner "A.2 — ActualizarCantidad: 500 → 450 plumas (muestras de retención)"
org1
invoke '{"function":"ActualizarCantidad","Args":["MED-001","450","Retiro de muestras de retencion y control de calidad"]}'
pause

banner "A.3 — Verificar el cambio (cantidad y txId)"
query '{"function":"LeerProducto","Args":["MED-001"]}'
pause

banner "A.4 — Historial de MED-001"
query '{"function":"ObtenerHistorial","Args":["MED-001"]}'
captura "Historial de MED-001 con la entrada de 450 plumas"
pause

# ═════════════════════════════════════════════════════════════════════════════
banner "B.1 — Pregunta 9: Org2 (no propietaria) intenta cambiar la cantidad de MED-001"
org2
debe_fallar '{"function":"ActualizarCantidad","Args":["MED-001","999","Intento de Org2"]}'
captura "Error de Org2 al actualizar MED-001"
pause

banner "B.2 — Cantidad no positiva (-10) desde el propietario"
org1
debe_fallar '{"function":"ActualizarCantidad","Args":["MED-001","-10","Prueba de validacion"]}'
pause

banner "B.3 — Pregunta 10: intentar ENTREGAR a la farmacia un lote que no está RECIBIDO"
org1
debe_fallar '{"function":"MarcarEntregado","Args":["MED-002","Drogueria Demo","Factura falsa"]}'
pause

# ═════════════════════════════════════════════════════════════════════════════
LOTE_FRIO="MED-FRIO-$SUFIJO"
banner "C — Cadena de frío rota: lote $LOTE_FRIO llega a 14.5 °C (rango 2–8 °C)"
org1
invoke "{\"function\":\"RegistrarProducto\",\"Args\":[\"$LOTE_FRIO\",\"Insulina glargina 100 UI/mL\",\"200\",\"plumas\",\"Planta Laboratorio Org1 - Barranquilla\",\"Farmacias de Corozal, Sucre\",\"5°C\",\"L-FRIO-$SUFIJO\",\"INVIMA 2026M-DEMO-011\",\"$VENCE\",\"2\",\"8\"]}"
invoke "{\"function\":\"TransferirProducto\",\"Args\":[\"$LOTE_FRIO\",\"Org2MSP\",\"Furgon refrigerado - falla del equipo de frio en ruta\"]}"
org2
invoke "{\"function\":\"ConfirmarRecepcion\",\"Args\":[\"$LOTE_FRIO\",\"14.5°C\",\"Datalogger registra excursion de temperatura\"]}"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$LOTE_FRIO\"]}"
echo ""
echo -e "  Intentando entregarlo a una farmacia..."
debe_fallar "{\"function\":\"MarcarEntregado\",\"Args\":[\"$LOTE_FRIO\",\"Drogueria Demo Corozal\",\"Factura DEMO-0999\"]}"
captura "alertaCadenaFrio=true y entrega bloqueada"
pause

# ═════════════════════════════════════════════════════════════════════════════
banner "D.1 — AgregarDocumento: certificado de análisis del lote MED-002"
org1
HASH=$(printf "Certificado de analisis lote L2026-AMX-014 - %s" "$SUFIJO" | shasum -a 256 2>/dev/null | awk '{print $1}')
[ -z "$HASH" ] && HASH=$(printf "Certificado de analisis lote L2026-AMX-014 - %s" "$SUFIJO" | sha256sum | awk '{print $1}')
echo "  SHA-256 del documento: $HASH"
invoke "{\"function\":\"AgregarDocumento\",\"Args\":[\"MED-002\",\"CERTIFICADO_ANALISIS\",\"$HASH\"]}"
query '{"function":"LeerProducto","Args":["MED-002"]}'
pause

banner "D.2 — Lotes que vencen en los próximos 90 días (rotación FEFO)"
query '{"function":"ConsultarPorVencer","Args":["90"]}'
pause

# ═════════════════════════════════════════════════════════════════════════════
banner "E — Pregunta 14: altura actual del ledger"
org1
peer channel getinfo -c "$CHANNEL"
captura "peer channel getinfo -c supplychannel"

echo -e "\n${GREEN}${BOLD}✓ Pruebas de las Partes 2 y 3 terminadas${NC}\n"
