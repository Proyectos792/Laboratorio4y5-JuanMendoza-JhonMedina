# Capstone Hyperledger Fabric: trazabilidad de medicamentos hacia farmacias

**Integrantes:** Juan Mendoza · Jhon Medina
**Base del laboratorio:** Jaider Reyes Herazo (chaincode `supplycc`, API REST y scripts originales)

Este proyecto adapta el Supply Chain del Laboratorio Capstone (Semanas 4 + 5) a una problemática concreta: **la cadena de suministro de medicamentos desde el laboratorio fabricante hasta las farmacias**. Los riesgos que ataca son los medicamentos falsificados, los lotes vencidos y las rupturas de la cadena de frío (por ejemplo, insulinas que deben conservarse entre 2 y 8 °C).

| Actor | Identidad en Fabric | Rol |
|---|---|---|
| Laboratorio fabricante (titular del registro INVIMA) | `Org1MSP` · peer0.org1:7051 | Registra lotes, ajusta cantidades, despacha |
| Distribuidor mayorista / operador logístico | `Org2MSP` · peer0.org2:9051 | Recibe, verifica temperatura, entrega a farmacias |
| Farmacia / droguería | Texto en `propietario` | Cliente final que recibe el lote |

Los nombres de farmacias y los registros sanitarios de los datos de ejemplo (`INVIMA 2026M-DEMO-xxx`) son **ficticios**.

## Estructura del repositorio

```text
.
├── chaincode/
│   ├── supplycc.go          ← chaincode en Go (12 funciones)
│   ├── go.mod
│   └── go.sum
├── api/
│   ├── app.js               ← API REST (Express + fabric-gateway)
│   ├── package.json
│   └── package-lock.json
├── set-env.sh               ← source ./set-env.sh org1 | org2
├── deploy-supplycc.sh       ← despliegue (versión 2.0, secuencia automática)
├── flujo-completo.sh        ← demostración: flujo completo del lote MED-010
├── pruebas-farmacia.sh      ← demostración: ActualizarCantidad, validaciones, cadena de frío, documentos
├── pruebas-api.sh           ← demostración: endpoints de la API REST
├── evidencias/              ← capturas de la ejecución real
└── README.md
```

## Cambios respecto al chaincode original

| Cambio | Para qué |
|---|---|
| Campos nuevos en `Producto`: `lote`, `registroInvima`, `fechaVencimiento`, `tempMin`, `tempMax`, `alertaCadenaFrio`, `documentos` | Representar un lote de medicamento con los datos que exige la regulación sanitaria |
| `RegistrarProducto` solo para `Org1MSP`; valida lote, registro INVIMA, rango térmico y fecha de vencimiento futura | Evitar que un tercero "fabrique" lotes en el ledger |
| `TransferirProducto` y `ConfirmarRecepcion` validan quién llama (propietario / destinatario) | Cerrar el hueco por el que cualquier organización podía mover un lote ajeno |
| `ConfirmarRecepcion` compara la temperatura recibida con el rango del lote; si está fuera → `alertaCadenaFrio = true` y evento `ExcursionTemperatura` | Detectar rupturas de la cadena de frío |
| `MarcarEntregado` bloquea lotes vencidos o en cuarentena; un lote en cuarentena solo puede devolverse a Org1 | Que un lote no apto nunca llegue a la farmacia |
| `ActualizarCantidad` solo permite disminuir y exige motivo | Un lote no crece después de fabricado; toda merma queda justificada |
| **Nueva** `AgregarDocumento(id, tipo, sha256)` | Anclar factura, certificado de análisis, guía, registro de temperatura o acta de recepción |
| **Nueva** `ConsultarPorVencer(dias)` | Apoyar la rotación FEFO en distribuidores y farmacias |
| Hora de la transacción con `GetTxTimestamp()` en lugar de `time.Now()` | Resultado idéntico en ambos peers endosantes (evita fallos de endorsement) |

**Funciones del chaincode:** `InitLedger`, `RegistrarProducto`, `TransferirProducto`, `ConfirmarRecepcion`, `ActualizarCantidad`, `MarcarEntregado`, `AgregarDocumento`, `LeerProducto`, `ObtenerHistorial`, `ConsultarTodos`, `ConsultarPorEstado`, `ConsultarPorVencer`.

## Entorno verificado

- Windows con **Ubuntu en WSL 2** y **Docker Desktop** (integración WSL activada).
- Fabric en `~/fabric/fabric-samples` (test-network con CouchDB), imágenes Fabric **2.5** (último parche).
- Go 1.21+, Node.js 18+, Python 3.

Los scripts buscan Fabric en `~/blockchain-lab/fabric-samples`. Si está en otra ruta, guárdenla una vez:

```bash
echo 'export FABRIC_DIR=$HOME/fabric/fabric-samples' >> ~/.bashrc
source ~/.bashrc
```

## 1. Levantar la red (si no está activa)

```bash
cd $FABRIC_DIR/test-network
./network.sh up createChannel -c supplychannel -s couchdb
```

Resultado esperado: 5 contenedores (`orderer.example.com`, `peer0.org1`, `peer0.org2`, `couchdb0`, `couchdb1`) y `peer channel getinfo -c supplychannel` con height 3.

## 2. Desplegar el chaincode

```bash
chmod +x *.sh
sed -i 's/\r$//' *.sh        # solo si el repo se clonó desde Windows
./deploy-supplycc.sh
```

El script detecta la secuencia confirmada en el canal (`querycommitted`) y usa la siguiente. En una red nueva usa **versión 2.0, secuencia 1**. Ejecuta package → install (Org1 y Org2) → approve (Org1 y Org2) → checkcommitreadiness → commit → `InitLedger`, que crea `MED-001` (insulina, 2–8 °C), `MED-002` (amoxicilina) y `MED-003` (acetaminofén, vence pronto).

Variables de entorno: `CC_VERSION=2.1` para redesplegar, `CC_SEQUENCE=N` para forzar una secuencia.

## 3. Demostraciones

```bash
source ./set-env.sh org1
./pruebas-farmacia.sh        # ActualizarCantidad, Org2 rechazada, cantidad negativa, cadena de frío, documentos
./flujo-completo.sh          # MED-010: registrar → ajustar → despachar → recibir → entregar (5 entradas de historial)
```

Ambos scripts hacen pausa en cada paso (`AUTO_CONTINUE=true` para ejecutarlos sin pausas). Comandos manuales equivalentes:

```bash
peer chaincode query -C supplychannel -n supplycc -c '{"function":"LeerProducto","Args":["MED-001"]}'
peer chaincode query -C supplychannel -n supplycc -c '{"function":"ObtenerHistorial","Args":["MED-001"]}'
peer channel getinfo -c supplychannel
```

CouchDB: `http://localhost:5984/_utils` (Org1) y `http://localhost:7984/_utils` (Org2), usuario `admin`, clave `adminpw`, base `supplychannel_supplycc`.

## 4. API REST

```bash
cd api
npm install
npm run org1          # Org1 en http://localhost:3000 (la terminal queda ocupada: es el servidor)
# en otra terminal:
npm run org2          # Org2 en http://localhost:3001
```

| Método | Ruta | Función del chaincode |
|---|---|---|
| GET | `/health` | `ConsultarTodos` (verifica la conexión real) |
| GET | `/productos` | `ConsultarTodos` |
| GET | `/productos/:id` | `LeerProducto` |
| GET | `/productos/:id/historial` | `ObtenerHistorial` |
| GET | `/productos/estado/:estado` | `ConsultarPorEstado` |
| GET | `/productos/alertas/por-vencer?dias=90` | `ConsultarPorVencer` |
| GET | `/productos/:id/documentos` | `LeerProducto` |
| POST | `/productos` | `RegistrarProducto` |
| PUT | `/productos/:id/transferir` | `TransferirProducto` |
| PUT | `/productos/:id/confirmar` | `ConfirmarRecepcion` |
| PUT | `/productos/:id/cantidad` | `ActualizarCantidad` |
| PUT | `/productos/:id/entregar` | `MarcarEntregado` |
| POST | `/productos/:id/documentos` | `AgregarDocumento` |

Ejemplo de registro:

```bash
curl -X POST http://localhost:3000/productos -H "Content-Type: application/json" -d '{
  "id":"MED-020","nombre":"Insulina glargina 100 UI/mL","cantidad":300,"unidad":"plumas",
  "origen":"Planta Laboratorio Org1 - Barranquilla","destino":"Farmacias de Sincelejo","temperatura":"5°C",
  "lote":"L2026-INS-020","registroInvima":"INVIMA 2026M-DEMO-020","fechaVencimiento":"2027-11-30",
  "tempMin":2,"tempMax":8}'
```

Secuencia completa de endpoints: `./pruebas-api.sh`.

## Resultados de la ejecución (16/11/2026)

| Prueba | Resultado |
|---|---|
| Despliegue | supplycc v2.0, secuencia 1; `checkcommitreadiness` Org1MSP **true**, Org2MSP **true** |
| `ActualizarCantidad` MED-001 | 500 → **450** plumas, txId `b7cb6376144ec57443fcdf4f2c2aba71ff0b26e2c993d3baf62809bb3c601ef3` |
| Org2 modifica MED-001 | Rechazado: "solo el propietario (Org1MSP) puede actualizar la cantidad" |
| Cantidad -10 | Rechazada: "la cantidad debe ser mayor a 0" |
| Entregar MED-002 sin recepción | Rechazado: "solo productos RECIBIDOS pueden marcarse como entregados" |
| MED-011 recibido a 14.5 °C (rango 2–8) | `alertaCadenaFrio = true`; entrega a farmacia rechazada (cuarentena) |
| `AgregarDocumento` MED-002 | Hash SHA-256 del certificado de análisis registrado |
| Flujo completo MED-010 | ENTREGADO a Drogueria Demo Centro - Sincelejo; historial con **5 entradas** |
| Height del canal | 3 → **17** (4 del despliegue + 5 de pruebas + 5 del flujo; lo rechazado no crea bloques) |
| API Org1 | `/health` conectado; POST MED-020 en el bloque 17; transferir y consultas OK |
| API Org2 | Confirmar MED-002 sin transferencia → error; MED-020 confirmado, con acta y ENTREGADO |

Las capturas están en [`evidencias/`](evidencias/).

## Problemas encontrados y solución

| Síntoma | Causa | Solución |
|---|---|---|
| `docker: command not found` en Ubuntu | Docker Desktop cerrado o integración WSL inactiva | Abrir Docker Desktop → Settings → Resources → WSL integration → activar Ubuntu |
| `chaincode install failed ... docker build failed ... write: broken pipe` | Incompatibilidad de Docker Engine v29 con imágenes antiguas de Fabric | Actualizar las imágenes al último parche 2.5 y recrear la red (ver abajo) |
| `namespace supplycc is not defined` | El chaincode aún no está desplegado en el canal | Normal antes del primer despliegue |
| `$'\r': command not found` | Scripts con finales de línea de Windows | `sed -i 's/\r$//' *.sh` |
| La API "se queda cargando" | Es el servidor escuchando peticiones | Dejarla abierta y usar otra terminal para `curl` |

Actualización de imágenes usada para el error de Docker v29:

```bash
for i in peer orderer ccenv baseos tools; do docker pull hyperledger/fabric-$i:2.5; done
for i in peer orderer tools; do docker tag hyperledger/fabric-$i:2.5 hyperledger/fabric-$i:latest; done
cd $FABRIC_DIR/test-network && ./network.sh down && ./network.sh up createChannel -c supplychannel -s couchdb
```
# Repositorio-Laboratorio-4-y-5-Juan-Mendoza-Jhon-Medina
# Laboratorio4y5-JuanMendoza-JhonMedina
