#!/bin/bash
# =============================================================================
#  flujo-completo.sh
#  Flujo completo de trazabilidad de un lote de MEDICAMENTO hasta la farmacia
#  Registrar → ActualizarCantidad → Transferir → Confirmar → MarcarEntregado
#  Laboratorio Capstone | Base: Jaider Reyes Herazo
#  Adaptación farmacéutica: Juan Mendoza y Jhon Medina
#
#  Uso:  ./flujo-completo.sh                    (lote MED-010, con pausas)
#        AUTO_CONTINUE=true ./flujo-completo.sh (sin pausas)
#        PRODUCT_ID=MED-015 ./flujo-completo.sh (otro identificador)
# =============================================================================

FABRIC_DIR="${FABRIC_DIR:-$HOME/blockchain-lab/fabric-samples}"
NETWORK_DIR="$FABRIC_DIR/test-network"
CHANNEL="supplychannel"
CC="supplycc"
PRODUCT_ID="${PRODUCT_ID:-MED-010}"
AUTO_CONTINUE="${AUTO_CONTINUE:-false}"
PAREJA="${PAREJA:-Juan Mendoza y Jhon Medina}"

# Fecha de vencimiento del lote demo: dentro de 18 meses (formato AAAA-MM-DD)
VENCE="$(date -v+18m +%Y-%m-%d 2>/dev/null || date -d '+18 months' +%Y-%m-%d)"

set -e

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'
BOLD='\033[1m'; NC='\033[0m'; RED='\033[0;31m'

export PATH="$FABRIC_DIR/bin:$PATH"
export FABRIC_CFG_PATH="$FABRIC_DIR/config/"
export CORE_PEER_TLS_ENABLED=true
export ORDERER_CA="$NETWORK_DIR/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem"
export PEER1_TLS="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem"
export PEER2_TLS="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/tlsca/tlsca.org2.example.com-cert.pem"

[ -x "$FABRIC_DIR/bin/peer" ] || { echo "ERROR: no se encontró $FABRIC_DIR/bin/peer" >&2; exit 1; }
[ -d "$NETWORK_DIR/organizations" ] || { echo "ERROR: la red no ha generado las organizaciones" >&2; exit 1; }

org1() {
  export CORE_PEER_LOCALMSPID="Org1MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE="$PEER1_TLS"
  export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
  export CORE_PEER_ADDRESS="localhost:7051"
}
org2() {
  export CORE_PEER_LOCALMSPID="Org2MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE="$PEER2_TLS"
  export CORE_PEER_MSPCONFIGPATH="$NETWORK_DIR/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
  export CORE_PEER_ADDRESS="localhost:9051"
}

# invoke: muestra el resultado (incluye el txid) y detiene el script si falla
invoke() {
  local out
  if ! out=$(peer chaincode invoke -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "$ORDERER_CA" \
    -C "$CHANNEL" -n "$CC" \
    --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER1_TLS" \
    --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER2_TLS" \
    --waitForEvent \
    -c "$1" 2>&1); then
    echo -e "${RED}$out${NC}" >&2
    exit 1
  fi
  echo "$out" | grep -E "txid|Chaincode invoke successful" | sed 's/^/  /' || true
}

query() {
  peer chaincode query -C "$CHANNEL" -n "$CC" -c "$1" 2>/dev/null | python3 -m json.tool
}

banner() { echo -e "\n${BOLD}${CYAN}▶ $1${NC}\n"; }
ok()     { echo -e "  ${GREEN}✓ $1${NC}"; }
pause()  {
  [ "$AUTO_CONTINUE" = "true" ] && return 0
  echo -e "\n${YELLOW}  Presiona ENTER para continuar...${NC}"
  read -r
}

clear 2>/dev/null || true
echo ""
echo "Capstone - $PAREJA - $(date)"
echo -e "${BOLD}╔═════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  Capstone — Trazabilidad de medicamentos a farmacias ║${NC}"
echo -e "${BOLD}║  Semanas 4 + 5 | Hyperledger Fabric | supplycc       ║${NC}"
echo -e "${BOLD}╚═════════════════════════════════════════════════════╝${NC}"
echo ""

# Si el lote ya existe (flujo ejecutado antes), usar un ID nuevo
org1
if peer chaincode query -C "$CHANNEL" -n "$CC" -c "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}" &>/dev/null; then
  NUEVO="$PRODUCT_ID-$(date +%H%M%S)"
  echo -e "${YELLOW}  $PRODUCT_ID ya existe en el ledger; esta ejecución usará $NUEVO${NC}"
  PRODUCT_ID="$NUEVO"
fi
LOTE="L$(date +%Y)-INS-$(date +%H%M)"

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 1 — Estado inicial del ledger (lotes registrados)"
org1
echo -e "  Consultando todos los lotes...\n"
query '{"function":"ConsultarTodos","Args":[]}'
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 2 — Org1 (Laboratorio) registra el lote $PRODUCT_ID de Insulina glargina"
org1
echo -e "  Lote $LOTE · 800 plumas · cadena de frío 2–8 °C · vence $VENCE\n"
invoke "{\"function\":\"RegistrarProducto\",\"Args\":[\"$PRODUCT_ID\",\"Insulina glargina 100 UI/mL (pluma 3 mL)\",\"800\",\"plumas\",\"Planta Laboratorio Org1 - Barranquilla\",\"Farmacias de Sincelejo, Sucre\",\"5°C\",\"$LOTE\",\"INVIMA 2026M-DEMO-010\",\"$VENCE\",\"2\",\"8\"]}"
ok "$PRODUCT_ID registrado en el ledger"
echo -e "\n  Verificando en el World State...\n"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 3 — Org1 ajusta la cantidad (muestras de retención / control de calidad)"
org1
echo -e "  Actualizando $PRODUCT_ID de 800 a 780 plumas (2.5 %)...\n"
invoke "{\"function\":\"ActualizarCantidad\",\"Args\":[\"$PRODUCT_ID\",\"780\",\"Retiro de muestras de retencion y control de calidad\"]}"
ok "Cantidad actualizada"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 4 — Org1 despacha el lote a Org2 (Distribuidor mayorista)"
org1
echo -e "  Transfiriendo $PRODUCT_ID a Org2MSP...\n"
invoke "{\"function\":\"TransferirProducto\",\"Args\":[\"$PRODUCT_ID\",\"Org2MSP\",\"Furgon refrigerado 2-8C Barranquilla-Sincelejo con datalogger - ETA 8 horas\"]}"
ok "Lote EN_TRANSITO hacia Org2"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 5 — Org2 confirma la recepción técnica del lote"
org2
echo -e "  Confirmando recepción desde Org2 con temperatura medida 5 °C...\n"
invoke "{\"function\":\"ConfirmarRecepcion\",\"Args\":[\"$PRODUCT_ID\",\"5°C\",\"Recepcion tecnica OK - datalogger sin excursiones - empaque y sellos integros\"]}"
ok "Recepción confirmada por Org2MSP (sin alerta de cadena de frío)"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 6 — Org2 entrega el lote a la farmacia (cliente final)"
org2
echo -e "  Entregando a Drogueria Demo Centro - Sincelejo...\n"
invoke "{\"function\":\"MarcarEntregado\",\"Args\":[\"$PRODUCT_ID\",\"Drogueria Demo Centro - Sincelejo\",\"Factura DEMO-2026-0457 - Acta de recepcion firmada por el regente de farmacia\"]}"
ok "Lote entregado a la farmacia"
query "{\"function\":\"LeerProducto\",\"Args\":[\"$PRODUCT_ID\"]}"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 7 — Historial inmutable del ledger para $PRODUCT_ID"
org1
echo -e "  Consultando historial (GetHistoryForKey) — se esperan 5 entradas...\n"
HIST=$(peer chaincode query -C "$CHANNEL" -n "$CC" -c "{\"function\":\"ObtenerHistorial\",\"Args\":[\"$PRODUCT_ID\"]}" 2>/dev/null)
echo "$HIST" | python3 -m json.tool
echo ""
ok "Entradas en el historial: $(echo "$HIST" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))')"
pause

# ─────────────────────────────────────────────────────────────────────────────
banner "ETAPA 8 — Consultas por estado"
org1
echo -e "  Lotes REGISTRADOS:\n"
query '{"function":"ConsultarPorEstado","Args":["REGISTRADO"]}'
echo ""
echo -e "  Lotes ENTREGADOS:\n"
query '{"function":"ConsultarPorEstado","Args":["ENTREGADO"]}'

echo ""
echo -e "${GREEN}${BOLD}╔═════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}${BOLD}║  ✓ Flujo completo de trazabilidad ejecutado          ║${NC}"
printf "${GREEN}${BOLD}║  Lote %-46s║${NC}\n" "$PRODUCT_ID verificado"
echo -e "${GREEN}${BOLD}║  Estado final: ENTREGADO a la farmacia               ║${NC}"
echo -e "${GREEN}${BOLD}╚═════════════════════════════════════════════════════╝${NC}"
echo ""
