#!/bin/bash
# =============================================================================
#  pruebas-api.sh — Parte 4: prueba los endpoints de la API REST
#  Requiere la API corriendo:  cd api && npm install && npm run org1   (puerto 3000)
#  Uso:  ./pruebas-api.sh                 (API de Org1 en localhost:3000)
#        API=http://localhost:3001 ./pruebas-api.sh
# =============================================================================
API="${API:-http://localhost:3000}"
ID="${ID:-MED-020}"
PAREJA="${PAREJA:-Juan Mendoza y Jhon Medina}"
VENCE="$(date -v+12m +%Y-%m-%d 2>/dev/null || date -d '+12 months' +%Y-%m-%d)"
AUTO_CONTINUE="${AUTO_CONTINUE:-false}"

CYAN='\033[0;36m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'

paso() { echo -e "\n${BOLD}${CYAN}▶ $1${NC}"; echo -e "${BOLD}\$ $2${NC}"; }
pause() { [ "$AUTO_CONTINUE" = "true" ] || { echo -e "${YELLOW}  ENTER para continuar...${NC}"; read -r; }; }
pretty() { python3 -m json.tool 2>/dev/null || cat; }

echo "Capstone API - $PAREJA - $(date)"

# Si el lote ya existe, usar otro ID para poder repetir la prueba
if curl -s -o /dev/null -w "%{http_code}" "$API/productos/$ID" | grep -q 200; then
  ID="$ID-$(date +%H%M%S)"
  echo -e "${YELLOW}  El lote base ya existe; se usará $ID${NC}"
fi

paso "GET /health" "curl $API/health"
curl -s "$API/health" | pretty; pause

paso "GET /productos" "curl $API/productos"
curl -s "$API/productos" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(f"{len(d)} lotes:", ", ".join(p["id"] for p in d))'; pause

paso "GET /productos/MED-010" "curl $API/productos/MED-010"
curl -s "$API/productos/MED-010" | pretty; pause

paso "GET /productos/MED-010/historial" "curl $API/productos/MED-010/historial"
curl -s "$API/productos/MED-010/historial" | pretty; pause

BODY="{\"id\":\"$ID\",\"nombre\":\"Insulina glargina 100 UI/mL (pluma 3 mL)\",\"cantidad\":300,\"unidad\":\"plumas\",\"origen\":\"Planta Laboratorio Org1 - Barranquilla\",\"destino\":\"Farmacias de Sincelejo\",\"temperatura\":\"5°C\",\"lote\":\"L2026-INS-020\",\"registroInvima\":\"INVIMA 2026M-DEMO-020\",\"fechaVencimiento\":\"$VENCE\",\"tempMin\":2,\"tempMax\":8}"
paso "POST /productos (registrar lote $ID)" "curl -X POST $API/productos -H 'Content-Type: application/json' -d '$BODY'"
curl -s -X POST "$API/productos" -H 'Content-Type: application/json' -d "$BODY" | pretty
curl -s "$API/productos/$ID" | pretty; pause

paso "PUT /productos/$ID/transferir" "curl -X PUT $API/productos/$ID/transferir -d '{\"nuevoPropietario\":\"Org2MSP\",\"detalle\":\"Furgon refrigerado Barranquilla-Sincelejo\"}'"
curl -s -X PUT "$API/productos/$ID/transferir" -H 'Content-Type: application/json' \
  -d '{"nuevoPropietario":"Org2MSP","detalle":"Furgon refrigerado 2-8C Barranquilla-Sincelejo"}' | pretty
curl -s "$API/productos/$ID" | python3 -c 'import sys,json; p=json.load(sys.stdin); print("estado:", p["estado"], "| propietario:", p["propietario"])'; pause

paso "GET /productos/$ID/historial" "curl $API/productos/$ID/historial"
curl -s "$API/productos/$ID/historial" | pretty; pause

paso "GET /productos/estado/EN_TRANSITO" "curl $API/productos/estado/EN_TRANSITO"
curl -s "$API/productos/estado/EN_TRANSITO" | pretty; pause

paso "PUT /productos/MED-002/cantidad (merma por avería)" "curl -X PUT $API/productos/MED-002/cantidad -d '{\"cantidad\":1180,\"motivo\":\"...\"}'"
curl -s -X PUT "$API/productos/MED-002/cantidad" -H 'Content-Type: application/json' \
  -d '{"cantidad":1180,"motivo":"Averia de 20 cajas en bodega del laboratorio"}' | pretty; pause

paso "GET /productos/alertas/por-vencer?dias=90" "curl '$API/productos/alertas/por-vencer?dias=90'"
curl -s "$API/productos/alertas/por-vencer?dias=90" | pretty; pause

paso "PUT /productos/$ID/entregar SIN haber confirmado recepción (debe fallar)" "curl -X PUT $API/productos/$ID/entregar ..."
curl -s -X PUT "$API/productos/$ID/entregar" -H 'Content-Type: application/json' \
  -d '{"clienteFinal":"Drogueria Demo","evidencia":"Factura DEMO"}' | pretty

echo -e "\n${BOLD}Para continuar como Org2 (confirmar y entregar):${NC}"
echo "  En otra terminal: cd api && npm run org2      (puerto 3001)"
echo "  curl -X PUT http://localhost:3001/productos/$ID/confirmar -H 'Content-Type: application/json' -d '{\"temperatura\":\"5°C\",\"detalle\":\"Recepcion tecnica OK\"}'"
echo "  curl -X POST http://localhost:3001/productos/$ID/documentos -H 'Content-Type: application/json' -d '{\"tipo\":\"ACTA_RECEPCION_TECNICA\",\"contenido\":\"Acta $ID\"}'"
echo "  curl -X PUT http://localhost:3001/productos/$ID/entregar -H 'Content-Type: application/json' -d '{\"clienteFinal\":\"Drogueria Demo Centro - Sincelejo\",\"evidencia\":\"Factura DEMO-2026-0500\"}'"
echo ""
