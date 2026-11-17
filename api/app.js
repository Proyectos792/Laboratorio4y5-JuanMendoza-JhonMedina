'use strict';
// =============================================================================
//  app.js — API REST Trazabilidad de Medicamentos | Capstone Semanas 4+5
//  Hyperledger Fabric + Express + fabric-gateway
//  Base: Jaider Reyes Herazo · Adaptación farmacéutica: Juan Mendoza y Jhon Medina
// =============================================================================

const express  = require('express');
const cors     = require('cors');
const morgan   = require('morgan');
const crypto   = require('crypto');
const fs       = require('fs');
const path     = require('path');
const grpc     = require('@grpc/grpc-js');
const { connect, hash, signers } = require('@hyperledger/fabric-gateway');

const app  = express();
const PORT = process.env.PORT || 3000;

// ── CONFIGURACIÓN FABRIC ─────────────────────────────────────────────────────
const FABRIC_DIR   = process.env.FABRIC_DIR || path.join(process.env.HOME, 'blockchain-lab', 'fabric-samples');
const NETWORK_DIR  = path.join(FABRIC_DIR, 'test-network');
const CHANNEL      = 'supplychannel';
const CHAINCODE    = 'supplycc';
const MSP_ID       = process.env.MSP_ID || 'Org1MSP';
const PEER_PORT    = MSP_ID === 'Org2MSP' ? '9051' : '7051';
const PEER_HOST    = MSP_ID === 'Org2MSP' ? 'peer0.org2.example.com' : 'peer0.org1.example.com';
const ORG_PATH     = MSP_ID === 'Org2MSP'
  ? path.join(NETWORK_DIR, 'organizations/peerOrganizations/org2.example.com')
  : path.join(NETWORK_DIR, 'organizations/peerOrganizations/org1.example.com');
const USER         = MSP_ID === 'Org2MSP' ? 'User1@org2.example.com' : 'User1@org1.example.com';

// ── MIDDLEWARE ────────────────────────────────────────────────────────────────
app.use(cors());
app.use(express.json());
app.use(morgan('dev'));

// ── INSTANCIA GLOBAL DEL CONTRATO ─────────────────────────────────────────────
let contract;

async function conectarFabric() {
  const tlsCertPath = path.join(ORG_PATH, `tlsca/tlsca.${MSP_ID === 'Org2MSP' ? 'org2' : 'org1'}.example.com-cert.pem`);
  const certDir     = path.join(ORG_PATH, `users/${USER}/msp/signcerts`);
  const keyDir      = path.join(ORG_PATH, `users/${USER}/msp/keystore`);

  const tlsCert = fs.readFileSync(tlsCertPath);
  const certFiles = fs.readdirSync(certDir);
  const certPem   = fs.readFileSync(path.join(certDir, certFiles[0]));
  const keyFiles  = fs.readdirSync(keyDir);
  const keyPem    = fs.readFileSync(path.join(keyDir, keyFiles[0]));

  const client = new grpc.Client(
    `localhost:${PEER_PORT}`,
    grpc.credentials.createSsl(tlsCert),
    { 'grpc.ssl_target_name_override': PEER_HOST }
  );

  const gateway = connect({
    client,
    identity: { mspId: MSP_ID, credentials: certPem },
    signer: signers.newPrivateKeySigner(
      crypto.createPrivateKey(keyPem)
    ),
    evaluateOptions: () => ({ deadline: Date.now() + 5000 }),
    endorseOptions:  () => ({ deadline: Date.now() + 15000 }),
    submitOptions:   () => ({ deadline: Date.now() + 5000 }),
    commitStatusOptions: () => ({ deadline: Date.now() + 60000 }),
  });

  const network = await gateway.getNetwork(CHANNEL);
  contract = network.getContract(CHAINCODE);
  console.log(`\n✅ Conectado a Fabric`);
  console.log(`   Org:       ${MSP_ID}`);
  console.log(`   Canal:     ${CHANNEL}`);
  console.log(`   Chaincode: ${CHAINCODE}\n`);
}

// ── HELPER ────────────────────────────────────────────────────────────────────
const decode = (result) => {
  const txt = Buffer.from(result).toString();
  return txt ? JSON.parse(txt) : null;
};

// Extrae el mensaje real del chaincode (fabric-gateway lo envía en err.details)
const mensajeFabric = (err) => {
  const detalles = (err.details || []).map((d) => d.message).filter(Boolean);
  if (detalles.length) return detalles.join(' | ');
  if (err.cause && err.cause.message) return err.cause.message;
  return err.message || 'Error interno';
};
const wrap   = (fn) => (req, res, next) => Promise.resolve(fn(req, res, next)).catch(next);

// =============================================================================
//  RUTAS API REST
// =============================================================================

// GET / — información rápida para abrir la API desde el navegador
app.get('/', (req, res) => {
  res.json({
    nombre: 'API Trazabilidad de Medicamentos — Hyperledger Fabric',
    org: MSP_ID,
    estado: 'activa',
    canal: CHANNEL,
    chaincode: CHAINCODE,
    endpoints: {
      salud: '/health',
      productos: '/productos',
      producto: '/productos/:id',
      historial: '/productos/:id/historial',
      porEstado: '/productos/estado/:estado',
      porVencer: '/productos/alertas/por-vencer?dias=90',
      documentos: '/productos/:id/documentos'
    }
  });
});

// GET /health — verifica la conexión real consultando el ledger
app.get('/health', wrap(async (req, res) => {
  const result = await contract.evaluateTransaction('ConsultarTodos');
  const productos = decode(result) || [];
  res.json({
    status: 'ok',
    fabric: 'conectado',
    org: MSP_ID,
    peer: `${PEER_HOST}:${PEER_PORT}`,
    canal: CHANNEL,
    chaincode: CHAINCODE,
    lotesEnLedger: productos.length,
    timestamp: new Date().toISOString()
  });
}));

// GET /productos — todos los productos
app.get('/productos', wrap(async (req, res) => {
  const result = await contract.evaluateTransaction('ConsultarTodos');
  res.json(decode(result) || []);
}));

// GET /productos/estado/:estado — filtrar por estado
app.get('/productos/estado/:estado', wrap(async (req, res) => {
  const { estado } = req.params;
  const result = await contract.evaluateTransaction('ConsultarPorEstado', estado.toUpperCase());
  res.json(decode(result) || []);
}));

// GET /productos/alertas/por-vencer?dias=90 — lotes próximos a vencer (FEFO)
app.get('/productos/alertas/por-vencer', wrap(async (req, res) => {
  const dias = parseInt(req.query.dias || '90', 10);
  const result = await contract.evaluateTransaction('ConsultarPorVencer', String(dias));
  res.json(decode(result) || []);
}));

// GET /productos/:id — producto por ID
app.get('/productos/:id', wrap(async (req, res) => {
  const result = await contract.evaluateTransaction('LeerProducto', req.params.id);
  res.json(decode(result));
}));

// GET /productos/:id/historial — historial del ledger
app.get('/productos/:id/historial', wrap(async (req, res) => {
  const result = await contract.evaluateTransaction('ObtenerHistorial', req.params.id);
  res.json(decode(result) || []);
}));

// GET /productos/:id/documentos — documentos soporte del lote
app.get('/productos/:id/documentos', wrap(async (req, res) => {
  const result = await contract.evaluateTransaction('LeerProducto', req.params.id);
  const producto = decode(result);
  res.json(producto.documentos || []);
}));

// POST /productos — registrar nuevo lote de medicamento
app.post('/productos', wrap(async (req, res) => {
  const {
    id, nombre, cantidad, unidad, origen, destino, temperatura,
    lote, registroInvima, fechaVencimiento, tempMin, tempMax
  } = req.body;
  const faltantes = ['id', 'nombre', 'cantidad', 'unidad', 'origen', 'destino',
    'lote', 'registroInvima', 'fechaVencimiento', 'tempMin', 'tempMax']
    .filter((c) => req.body[c] === undefined || req.body[c] === '');
  if (faltantes.length) {
    return res.status(400).json({ error: `Campos requeridos faltantes: ${faltantes.join(', ')}` });
  }
  const result = await contract.submitAsync(
    'RegistrarProducto',
    {
      arguments: [
        id, nombre, String(cantidad), unidad, origen, destino, temperatura || `${tempMin}-${tempMax}°C`,
        lote, registroInvima, fechaVencimiento, String(tempMin), String(tempMax)
      ]
    }
  );
  const status = await result.getStatus();
  if (!status.successful) {
    return res.status(409).json({ error: `Transacción ${status.transactionId} inválida (código ${status.code})` });
  }
  res.status(201).json({
    message: `Lote ${id} (${lote}) registrado en el ledger`,
    txId: status.transactionId,
    bloque: String(status.blockNumber),
    timestamp: new Date().toISOString()
  });
}));

// PUT /productos/:id/transferir — transferir a otra org
app.put('/productos/:id/transferir', wrap(async (req, res) => {
  const { nuevoPropietario, detalle } = req.body;
  if (!nuevoPropietario || !detalle) {
    return res.status(400).json({ error: 'Campos requeridos: nuevoPropietario, detalle' });
  }
  await contract.submitTransaction('TransferirProducto', req.params.id, nuevoPropietario, detalle);
  res.json({ message: `Producto ${req.params.id} transferido a ${nuevoPropietario}` });
}));

// PUT /productos/:id/confirmar — confirmar recepción
app.put('/productos/:id/confirmar', wrap(async (req, res) => {
  const { temperatura, detalle } = req.body;
  await contract.submitTransaction(
    'ConfirmarRecepcion',
    req.params.id,
    temperatura || 'N/A',
    detalle || 'Recepción confirmada'
  );
  res.json({ message: `Recepción del producto ${req.params.id} confirmada` });
}));

// PUT /productos/:id/cantidad — actualizar cantidad
app.put('/productos/:id/cantidad', wrap(async (req, res) => {
  const { cantidad, motivo } = req.body;
  if (!cantidad || !motivo) {
    return res.status(400).json({ error: 'Campos requeridos: cantidad, motivo' });
  }
  await contract.submitTransaction('ActualizarCantidad', req.params.id, String(cantidad), motivo);
  res.json({ message: `Cantidad del producto ${req.params.id} actualizada a ${cantidad}` });
}));

// PUT /productos/:id/entregar — marcar como entregado
app.put('/productos/:id/entregar', wrap(async (req, res) => {
  const { clienteFinal, evidencia } = req.body;
  if (!clienteFinal || !evidencia) {
    return res.status(400).json({ error: 'Campos requeridos: clienteFinal, evidencia' });
  }
  await contract.submitTransaction('MarcarEntregado', req.params.id, clienteFinal, evidencia);
  res.json({ message: `Producto ${req.params.id} marcado como ENTREGADO a ${clienteFinal}` });
}));

// POST /productos/:id/documentos — adjuntar hash SHA-256 de un documento soporte
app.post('/productos/:id/documentos', wrap(async (req, res) => {
  const { tipo, hash: hashDoc, contenido } = req.body;
  // Se puede enviar el hash ya calculado o el contenido (texto) para calcularlo aquí
  const huella = hashDoc || (contenido ? crypto.createHash('sha256').update(contenido).digest('hex') : '');
  if (!tipo || !huella) {
    return res.status(400).json({ error: 'Campos requeridos: tipo y (hash o contenido)' });
  }
  await contract.submitTransaction('AgregarDocumento', req.params.id, tipo, huella);
  res.status(201).json({ message: `Documento ${tipo} asociado a ${req.params.id}`, sha256: huella });
}));

// ── MANEJO DE ERRORES ─────────────────────────────────────────────────────────
app.use((err, req, res, next) => {
  const msg = mensajeFabric(err);
  console.error('❌', msg);
  let status = 500;
  if (msg.includes('no encontrado')) status = 404;
  else if (/solo |inválid|obligatori|debe |ya existe|ya fue|ya está|no está|cuarentena|venci/i.test(msg)) status = 400;
  res.status(status).json({ error: msg });
});

app.use((req, res) => res.status(404).json({ error: `Ruta no encontrada: ${req.method} ${req.url}` }));

// =============================================================================
//  ARRANQUE
// =============================================================================
async function main() {
  try {
    await conectarFabric();
    app.listen(PORT, () => {
      console.log(`🚀 API Trazabilidad de Medicamentos en http://localhost:${PORT}`);
      console.log('\n📋 Endpoints disponibles:');
      console.log('   GET    /health');
      console.log('   GET    /productos');
      console.log('   GET    /productos/:id');
      console.log('   GET    /productos/:id/historial');
      console.log('   GET    /productos/estado/:estado');
      console.log('   GET    /productos/alertas/por-vencer?dias=90');
      console.log('   GET    /productos/:id/documentos');
      console.log('   POST   /productos');
      console.log('   PUT    /productos/:id/transferir');
      console.log('   PUT    /productos/:id/confirmar');
      console.log('   PUT    /productos/:id/cantidad');
      console.log('   PUT    /productos/:id/entregar');
      console.log('   POST   /productos/:id/documentos\n');
    });
  } catch (err) {
    console.error('❌ Error conectando a Fabric:', err);
    process.exit(1);
  }
}

main();
