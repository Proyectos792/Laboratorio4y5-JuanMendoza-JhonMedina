package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// =============================================================================
//  supplycc.go — Chaincode de trazabilidad de MEDICAMENTOS hacia farmacias
//  Laboratorio Capstone Semanas 4 + 5 | Hyperledger Fabric — Canal: supplychannel
//  Base: Jaider Reyes Herazo · Adaptación farmacéutica: Juan Mendoza y Jhon Medina
//
//  Problemática: medicamentos falsificados, lotes vencidos o expuestos a
//  rupturas de la cadena de frío que llegan a las farmacias sin que nadie
//  pueda demostrar por dónde pasaron.
//
//  Actores del canal:
//    Org1MSP → Laboratorio fabricante (titular del registro sanitario INVIMA)
//    Org2MSP → Distribuidor mayorista / operador logístico farmacéutico
//    Cliente final (texto) → Farmacia o droguería que recibe el lote
// =============================================================================

// Estados válidos del ciclo de vida de un lote
const (
	EstadoRegistrado = "REGISTRADO"
	EstadoEnTransito = "EN_TRANSITO"
	EstadoRecibido   = "RECIBIDO"
	EstadoEntregado  = "ENTREGADO"

	// MSP autorizado para registrar lotes nuevos (el fabricante)
	MSPFabricante = "Org1MSP"

	formatoFecha = "2006-01-02"
)

// Tipos de documento soporte permitidos por AgregarDocumento
var tiposDocumento = map[string]bool{
	"FACTURA":                true,
	"CERTIFICADO_ANALISIS":   true,
	"GUIA_REMISION":          true,
	"REGISTRO_TEMPERATURA":   true,
	"ACTA_RECEPCION_TECNICA": true,
}

// Documento referencia un soporte externo por su hash SHA-256 (el archivo NO
// se guarda en el ledger; solo su huella para poder verificarlo después).
type Documento struct {
	Tipo          string `json:"tipo"`
	Hash          string `json:"hash"`
	RegistradoPor string `json:"registradoPor"`
	Timestamp     string `json:"timestamp"`
	TxID          string `json:"txId"`
}

// Producto representa un LOTE de medicamento en la cadena de suministro
type Producto struct {
	// Campos originales del laboratorio
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Cantidad    int    `json:"cantidad"`
	Unidad      string `json:"unidad"`
	Propietario string `json:"propietario"`
	Estado      string `json:"estado"`
	Origen      string `json:"origen"`
	Destino     string `json:"destino"`
	Temperatura string `json:"temperatura"`
	Timestamp   string `json:"timestamp"`
	TxID        string `json:"txId"`

	// Campos agregados para el contexto farmacéutico
	Lote             string      `json:"lote"`
	RegistroInvima   string      `json:"registroInvima"`
	FechaVencimiento string      `json:"fechaVencimiento"` // AAAA-MM-DD
	TempMin          float64     `json:"tempMin"`          // °C mínimo permitido
	TempMax          float64     `json:"tempMax"`          // °C máximo permitido
	AlertaCadenaFrio bool        `json:"alertaCadenaFrio"` // true = lote en cuarentena
	Documentos       []Documento `json:"documentos"`
}

// EventoHistorial registra cada cambio de estado en el ledger
type EventoHistorial struct {
	TxID      string `json:"txId"`
	Timestamp string `json:"timestamp"`
	Estado    string `json:"estado"`
	Actor     string `json:"actor"`
	Detalle   string `json:"detalle"`
}

// SupplyContract implementa ContractInterface
type SupplyContract struct {
	contractapi.Contract
}

// =============================================================================
//
//	FUNCIÓN 1 — InitLedger
//	Carga tres lotes de ejemplo de un laboratorio de la región Caribe
//
// =============================================================================
func (sc *SupplyContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	ts, err := txTime(ctx)
	if err != nil {
		return err
	}
	txID := ctx.GetStub().GetTxID()
	ahora := ts.Format(time.RFC3339)

	productos := []Producto{
		{
			ID: "MED-001", Nombre: "Insulina glargina 100 UI/mL (pluma 3 mL)", Cantidad: 500,
			Unidad: "plumas", Propietario: MSPFabricante, Estado: EstadoRegistrado,
			Origen: "Planta Laboratorio Org1 - Barranquilla", Destino: "CEDI Distribuidor - Sincelejo",
			Temperatura: "5°C", Lote: "L2026-INS-001", RegistroInvima: "INVIMA 2026M-DEMO-001",
			FechaVencimiento: "2027-08-31", TempMin: 2, TempMax: 8,
		},
		{
			ID: "MED-002", Nombre: "Amoxicilina 500 mg capsulas x 50", Cantidad: 1200,
			Unidad: "cajas", Propietario: MSPFabricante, Estado: EstadoRegistrado,
			Origen: "Planta Laboratorio Org1 - Barranquilla", Destino: "CEDI Distribuidor - Monteria",
			Temperatura: "25°C", Lote: "L2026-AMX-014", RegistroInvima: "INVIMA 2026M-DEMO-002",
			FechaVencimiento: "2028-03-31", TempMin: 15, TempMax: 30,
		},
		{
			ID: "MED-003", Nombre: "Acetaminofen 500 mg tabletas x 100", Cantidad: 3000,
			Unidad: "cajas", Propietario: MSPFabricante, Estado: EstadoRegistrado,
			Origen: "Planta Laboratorio Org1 - Barranquilla", Destino: "CEDI Distribuidor - Cartagena",
			Temperatura: "24°C", Lote: "L2025-ACE-087", RegistroInvima: "INVIMA 2026M-DEMO-003",
			FechaVencimiento: "2027-01-31", TempMin: 15, TempMax: 30,
		},
	}

	for _, p := range productos {
		p.Timestamp = ahora
		p.TxID = txID
		p.Documentos = []Documento{}
		pJSON, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("error serializando %s: %v", p.ID, err)
		}
		if err := ctx.GetStub().PutState(p.ID, pJSON); err != nil {
			return fmt.Errorf("error guardando %s: %v", p.ID, err)
		}
	}
	return nil
}

// =============================================================================
//
//	FUNCIÓN 2 — RegistrarProducto
//	El laboratorio fabricante (Org1) registra un lote nuevo
//
// =============================================================================
func (sc *SupplyContract) RegistrarProducto(
	ctx contractapi.TransactionContextInterface,
	id, nombre string, cantidad int,
	unidad, origen, destino, temperatura string,
	lote, registroInvima, fechaVencimiento string,
	tempMin, tempMax float64,
) error {
	existe, err := sc.existeProducto(ctx, id)
	if err != nil {
		return err
	}
	if existe {
		return fmt.Errorf("el producto %s ya existe en el ledger", id)
	}

	clientID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("no se pudo leer el MSP del llamante: %v", err)
	}
	if clientID != MSPFabricante {
		return fmt.Errorf("solo el laboratorio fabricante (%s) puede registrar lotes — llamante: %s", MSPFabricante, clientID)
	}

	// Validaciones farmacéuticas
	if cantidad <= 0 {
		return fmt.Errorf("la cantidad debe ser mayor a 0, recibido: %d", cantidad)
	}
	if strings.TrimSpace(lote) == "" {
		return fmt.Errorf("el número de lote es obligatorio")
	}
	if strings.TrimSpace(registroInvima) == "" {
		return fmt.Errorf("el registro sanitario INVIMA es obligatorio")
	}
	if tempMin >= tempMax {
		return fmt.Errorf("rango de temperatura inválido: mínimo %.1f°C debe ser menor que máximo %.1f°C", tempMin, tempMax)
	}
	ts, err := txTime(ctx)
	if err != nil {
		return err
	}
	vence, err := time.Parse(formatoFecha, fechaVencimiento)
	if err != nil {
		return fmt.Errorf("fechaVencimiento debe tener formato AAAA-MM-DD, recibido: %s", fechaVencimiento)
	}
	if !vence.After(ts) {
		return fmt.Errorf("no se puede registrar un lote vencido (vence %s)", fechaVencimiento)
	}

	producto := Producto{
		ID: id, Nombre: nombre, Cantidad: cantidad,
		Unidad: unidad, Propietario: clientID, Estado: EstadoRegistrado,
		Origen: origen, Destino: destino, Temperatura: temperatura,
		Timestamp: ts.Format(time.RFC3339),
		TxID:      ctx.GetStub().GetTxID(),
		Lote:      lote, RegistroInvima: registroInvima, FechaVencimiento: fechaVencimiento,
		TempMin: tempMin, TempMax: tempMax, Documentos: []Documento{},
	}

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}

	if err := ctx.GetStub().SetEvent("ProductoRegistrado", pJSON); err != nil {
		return err
	}
	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 3 — TransferirProducto
//	El propietario actual despacha el lote a otra organización
//
// =============================================================================
func (sc *SupplyContract) TransferirProducto(
	ctx contractapi.TransactionContextInterface,
	id, nuevoPropietario, detalle string,
) error {
	producto, err := sc.LeerProducto(ctx, id)
	if err != nil {
		return err
	}

	if producto.Estado == EstadoEntregado {
		return fmt.Errorf("el producto %s ya fue entregado y no puede transferirse", id)
	}
	if producto.Estado == EstadoEnTransito {
		return fmt.Errorf("el producto %s ya está EN_TRANSITO hacia %s", id, producto.Propietario)
	}

	clientID, _ := ctx.GetClientIdentity().GetMSPID()
	if clientID != producto.Propietario {
		return fmt.Errorf("solo el propietario (%s) puede despachar el lote — llamante: %s", producto.Propietario, clientID)
	}
	if nuevoPropietario == producto.Propietario {
		return fmt.Errorf("el nuevo propietario debe ser distinto del actual (%s)", producto.Propietario)
	}

	ts, err := txTime(ctx)
	if err != nil {
		return err
	}
	if sc.estaVencido(producto, ts) {
		return fmt.Errorf("el lote %s venció el %s y no puede despacharse", producto.Lote, producto.FechaVencimiento)
	}
	// Un lote en cuarentena solo puede devolverse al fabricante (para análisis o destrucción)
	if producto.AlertaCadenaFrio && nuevoPropietario != MSPFabricante {
		return fmt.Errorf("el lote %s está en cuarentena por excursión de temperatura: solo puede devolverse a %s", producto.Lote, MSPFabricante)
	}

	evento := EventoHistorial{
		TxID: ctx.GetStub().GetTxID(), Timestamp: ts.Format(time.RFC3339),
		Estado: EstadoEnTransito, Actor: producto.Propietario, Detalle: detalle,
	}
	if err := sc.guardarHistorial(ctx, id, evento); err != nil {
		return err
	}

	producto.Propietario = nuevoPropietario
	producto.Estado = EstadoEnTransito
	producto.TxID = ctx.GetStub().GetTxID()
	producto.Timestamp = ts.Format(time.RFC3339)

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}

	if err := ctx.GetStub().SetEvent("ProductoTransferido", pJSON); err != nil {
		return err
	}
	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 4 — ConfirmarRecepcion
//	El distribuidor (Org2) confirma que recibió el lote y reporta la
//	temperatura medida. Si está fuera del rango → alerta de cadena de frío.
//
// =============================================================================
func (sc *SupplyContract) ConfirmarRecepcion(
	ctx contractapi.TransactionContextInterface,
	id, temperaturaRecibido, detalle string,
) error {
	producto, err := sc.LeerProducto(ctx, id)
	if err != nil {
		return err
	}

	if producto.Estado != EstadoEnTransito {
		return fmt.Errorf("el producto %s no está EN_TRANSITO (estado: %s)", id, producto.Estado)
	}

	clientID, _ := ctx.GetClientIdentity().GetMSPID()
	if clientID != producto.Propietario {
		return fmt.Errorf("solo el destinatario (%s) puede confirmar la recepción — llamante: %s", producto.Propietario, clientID)
	}

	temp, err := parseTemperatura(temperaturaRecibido)
	if err != nil {
		return err
	}
	ts, err := txTime(ctx)
	if err != nil {
		return err
	}

	fueraDeRango := temp < producto.TempMin || temp > producto.TempMax
	detalleEvento := fmt.Sprintf("%s — Temp recibido: %s (rango %.1f–%.1f°C)", detalle, temperaturaRecibido, producto.TempMin, producto.TempMax)
	if fueraDeRango {
		detalleEvento += " — EXCURSIÓN DE TEMPERATURA: lote en cuarentena"
	}

	evento := EventoHistorial{
		TxID: ctx.GetStub().GetTxID(), Timestamp: ts.Format(time.RFC3339),
		Estado: EstadoRecibido, Actor: clientID, Detalle: detalleEvento,
	}
	if err := sc.guardarHistorial(ctx, id, evento); err != nil {
		return err
	}

	producto.Estado = EstadoRecibido
	producto.Temperatura = temperaturaRecibido
	producto.AlertaCadenaFrio = producto.AlertaCadenaFrio || fueraDeRango
	producto.TxID = ctx.GetStub().GetTxID()
	producto.Timestamp = ts.Format(time.RFC3339)

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}

	// Un evento por transacción: si hubo excursión se emite la alerta (más crítica)
	nombreEvento := "RecepcionConfirmada"
	if fueraDeRango {
		nombreEvento = "ExcursionTemperatura"
	}
	if err := ctx.GetStub().SetEvent(nombreEvento, pJSON); err != nil {
		return err
	}
	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 5 — ActualizarCantidad
//	FUNCIÓN MODIFICADA EN EL LABORATORIO — validación de propietario,
//	cantidad positiva, registro en historial y evento blockchain
//
// =============================================================================
func (sc *SupplyContract) ActualizarCantidad(
	ctx contractapi.TransactionContextInterface,
	id string, nuevaCantidad int, motivo string,
) error {
	producto, err := sc.LeerProducto(ctx, id)
	if err != nil {
		return err
	}

	// Validar que quien actualiza es el propietario actual
	clientID, _ := ctx.GetClientIdentity().GetMSPID()
	if clientID != producto.Propietario {
		return fmt.Errorf("solo el propietario (%s) puede actualizar la cantidad — llamante: %s",
			producto.Propietario, clientID)
	}

	// Validar que la cantidad sea positiva
	if nuevaCantidad <= 0 {
		return fmt.Errorf("la cantidad debe ser mayor a 0, recibido: %d", nuevaCantidad)
	}

	// Un lote no "crece" después de fabricado: solo se permiten mermas
	if nuevaCantidad > producto.Cantidad {
		return fmt.Errorf("la cantidad de un lote solo puede disminuir (actual %d, solicitada %d)", producto.Cantidad, nuevaCantidad)
	}

	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("el motivo del ajuste es obligatorio")
	}

	ts, err := txTime(ctx)
	if err != nil {
		return err
	}

	// Registrar en historial
	evento := EventoHistorial{
		TxID:      ctx.GetStub().GetTxID(),
		Timestamp: ts.Format(time.RFC3339),
		Estado:    producto.Estado,
		Actor:     clientID,
		Detalle:   fmt.Sprintf("Cantidad actualizada de %d a %d %s — Motivo: %s", producto.Cantidad, nuevaCantidad, producto.Unidad, motivo),
	}
	if err := sc.guardarHistorial(ctx, id, evento); err != nil {
		return err
	}

	cantidadAnterior := producto.Cantidad
	producto.Cantidad = nuevaCantidad
	producto.TxID = ctx.GetStub().GetTxID()
	producto.Timestamp = ts.Format(time.RFC3339)

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}

	// Emitir evento con información de la actualización
	eventoData := map[string]interface{}{
		"id":               id,
		"lote":             producto.Lote,
		"cantidadAnterior": cantidadAnterior,
		"cantidadNueva":    nuevaCantidad,
		"unidad":           producto.Unidad,
		"motivo":           motivo,
		"actor":            clientID,
	}
	eventoJSON, _ := json.Marshal(eventoData)
	if err := ctx.GetStub().SetEvent("CantidadActualizada", eventoJSON); err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 6 — MarcarEntregado
//	El distribuidor entrega el lote a la farmacia (cliente final).
//	Bloquea lotes vencidos o en cuarentena por cadena de frío.
//
// =============================================================================
func (sc *SupplyContract) MarcarEntregado(
	ctx contractapi.TransactionContextInterface,
	id, clienteFinal, evidencia string,
) error {
	producto, err := sc.LeerProducto(ctx, id)
	if err != nil {
		return err
	}

	// Solo productos RECIBIDOS pueden marcarse como ENTREGADOS
	if producto.Estado != EstadoRecibido {
		return fmt.Errorf("solo productos RECIBIDOS pueden marcarse como entregados (estado actual: %s)", producto.Estado)
	}

	clientID, _ := ctx.GetClientIdentity().GetMSPID()
	if clientID != producto.Propietario {
		return fmt.Errorf("solo quien tiene el lote (%s) puede entregarlo — llamante: %s", producto.Propietario, clientID)
	}
	if strings.TrimSpace(clienteFinal) == "" || strings.TrimSpace(evidencia) == "" {
		return fmt.Errorf("clienteFinal y evidencia son obligatorios")
	}

	ts, err := txTime(ctx)
	if err != nil {
		return err
	}
	if sc.estaVencido(producto, ts) {
		return fmt.Errorf("el lote %s venció el %s: no puede entregarse a la farmacia", producto.Lote, producto.FechaVencimiento)
	}
	if producto.AlertaCadenaFrio {
		return fmt.Errorf("el lote %s está en cuarentena por excursión de temperatura: no puede entregarse a la farmacia", producto.Lote)
	}

	evento := EventoHistorial{
		TxID:      ctx.GetStub().GetTxID(),
		Timestamp: ts.Format(time.RFC3339),
		Estado:    EstadoEntregado,
		Actor:     clientID,
		Detalle:   fmt.Sprintf("Entregado a: %s — Evidencia: %s", clienteFinal, evidencia),
	}
	if err := sc.guardarHistorial(ctx, id, evento); err != nil {
		return err
	}

	producto.Estado = EstadoEntregado
	producto.Propietario = clienteFinal
	producto.TxID = ctx.GetStub().GetTxID()
	producto.Timestamp = ts.Format(time.RFC3339)

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}

	if err := ctx.GetStub().SetEvent("ProductoEntregado", pJSON); err != nil {
		return err
	}
	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 7 — AgregarDocumento (FUNCIÓN NUEVA — Pregunta 12)
//	Asocia al lote la huella SHA-256 de un documento soporte
//	(factura, certificado de análisis, guía de remisión, registro de
//	temperatura del datalogger o acta de recepción técnica).
//
// =============================================================================
func (sc *SupplyContract) AgregarDocumento(
	ctx contractapi.TransactionContextInterface,
	id, tipoDoc, contenidoHash string,
) error {
	producto, err := sc.LeerProducto(ctx, id)
	if err != nil {
		return err
	}

	tipoDoc = strings.ToUpper(strings.TrimSpace(tipoDoc))
	if !tiposDocumento[tipoDoc] {
		return fmt.Errorf("tipo de documento inválido: %s. Válidos: FACTURA, CERTIFICADO_ANALISIS, GUIA_REMISION, REGISTRO_TEMPERATURA, ACTA_RECEPCION_TECNICA", tipoDoc)
	}

	contenidoHash = strings.ToLower(strings.TrimSpace(contenidoHash))
	if b, err := hex.DecodeString(contenidoHash); err != nil || len(b) != 32 {
		return fmt.Errorf("contenidoHash debe ser un SHA-256 en hexadecimal (64 caracteres)")
	}

	clientID, _ := ctx.GetClientIdentity().GetMSPID()
	if clientID != producto.Propietario {
		return fmt.Errorf("solo el propietario actual (%s) puede adjuntar documentos — llamante: %s", producto.Propietario, clientID)
	}
	if producto.Estado == EstadoEntregado {
		return fmt.Errorf("el lote %s ya fue entregado; no admite documentos nuevos", id)
	}
	for _, d := range producto.Documentos {
		if d.Hash == contenidoHash {
			return fmt.Errorf("ese documento ya está registrado para %s (tipo %s)", id, d.Tipo)
		}
	}

	ts, err := txTime(ctx)
	if err != nil {
		return err
	}
	doc := Documento{
		Tipo: tipoDoc, Hash: contenidoHash, RegistradoPor: clientID,
		Timestamp: ts.Format(time.RFC3339), TxID: ctx.GetStub().GetTxID(),
	}

	evento := EventoHistorial{
		TxID: doc.TxID, Timestamp: doc.Timestamp, Estado: producto.Estado, Actor: clientID,
		Detalle: fmt.Sprintf("Documento %s agregado — SHA-256: %s", tipoDoc, contenidoHash),
	}
	if err := sc.guardarHistorial(ctx, id, evento); err != nil {
		return err
	}

	producto.Documentos = append(producto.Documentos, doc)
	producto.TxID = doc.TxID
	producto.Timestamp = doc.Timestamp

	pJSON, err := json.Marshal(producto)
	if err != nil {
		return err
	}
	docJSON, _ := json.Marshal(map[string]interface{}{"id": id, "documento": doc})
	if err := ctx.GetStub().SetEvent("DocumentoAgregado", docJSON); err != nil {
		return err
	}
	return ctx.GetStub().PutState(id, pJSON)
}

// =============================================================================
//
//	FUNCIÓN 8 — LeerProducto
//	Consulta el estado actual de un lote (World State)
//
// =============================================================================
func (sc *SupplyContract) LeerProducto(
	ctx contractapi.TransactionContextInterface, id string,
) (*Producto, error) {
	pJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("error leyendo %s: %v", id, err)
	}
	if pJSON == nil {
		return nil, fmt.Errorf("producto %s no encontrado en el ledger", id)
	}
	var producto Producto
	if err := json.Unmarshal(pJSON, &producto); err != nil {
		return nil, err
	}
	if producto.Documentos == nil {
		producto.Documentos = []Documento{}
	}
	return &producto, nil
}

// =============================================================================
//
//	FUNCIÓN 9 — ObtenerHistorial
//	Retorna todas las versiones del lote desde el ledger (GetHistoryForKey)
//
// =============================================================================
func (sc *SupplyContract) ObtenerHistorial(
	ctx contractapi.TransactionContextInterface, id string,
) ([]*EventoHistorial, error) {
	iterator, err := ctx.GetStub().GetHistoryForKey(id)
	if err != nil {
		return nil, err
	}
	defer iterator.Close()

	historial := []*EventoHistorial{}
	for iterator.HasNext() {
		respuesta, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var producto Producto
		if err := json.Unmarshal(respuesta.Value, &producto); err != nil {
			continue
		}
		detalle := fmt.Sprintf("Lote: %s | Cantidad: %d %s | Temperatura: %s | Documentos: %d",
			producto.Lote, producto.Cantidad, producto.Unidad, producto.Temperatura, len(producto.Documentos))
		if producto.AlertaCadenaFrio {
			detalle += " | ALERTA CADENA DE FRÍO"
		}
		evento := &EventoHistorial{
			TxID:      respuesta.TxId,
			Timestamp: time.Unix(respuesta.Timestamp.Seconds, 0).UTC().Format(time.RFC3339),
			Estado:    producto.Estado,
			Actor:     producto.Propietario,
			Detalle:   detalle,
		}
		historial = append(historial, evento)
	}
	return historial, nil
}

// =============================================================================
//
//	FUNCIÓN 10 — ConsultarTodos
//	Recorre el World State (funciona con LevelDB y CouchDB)
//
// =============================================================================
func (sc *SupplyContract) ConsultarTodos(
	ctx contractapi.TransactionContextInterface,
) ([]*Producto, error) {
	iterator, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()

	productos := []*Producto{}
	for iterator.HasNext() {
		resp, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var producto Producto
		if err := json.Unmarshal(resp.Value, &producto); err != nil || producto.ID == "" {
			continue
		}
		if producto.Documentos == nil {
			producto.Documentos = []Documento{}
		}
		productos = append(productos, &producto)
	}
	return productos, nil
}

// =============================================================================
//
//	FUNCIÓN 11 — ConsultarPorEstado
//
// =============================================================================
func (sc *SupplyContract) ConsultarPorEstado(
	ctx contractapi.TransactionContextInterface, estado string,
) ([]*Producto, error) {
	estados := map[string]bool{
		EstadoRegistrado: true, EstadoEnTransito: true,
		EstadoRecibido: true, EstadoEntregado: true,
	}
	if !estados[estado] {
		return nil, fmt.Errorf("estado inválido: %s. Válidos: REGISTRADO, EN_TRANSITO, RECIBIDO, ENTREGADO", estado)
	}
	productos, err := sc.ConsultarTodos(ctx)
	if err != nil {
		return nil, err
	}
	filtrados := []*Producto{}
	for _, producto := range productos {
		if producto.Estado == estado {
			filtrados = append(filtrados, producto)
		}
	}
	return filtrados, nil
}

// =============================================================================
//
//	FUNCIÓN 12 — ConsultarPorVencer (NUEVA)
//	Lotes aún no entregados que vencen dentro de N días (apoya la
//	rotación FEFO: "primero en vencer, primero en salir")
//
// =============================================================================
func (sc *SupplyContract) ConsultarPorVencer(
	ctx contractapi.TransactionContextInterface, dias int,
) ([]*Producto, error) {
	if dias <= 0 {
		return nil, fmt.Errorf("dias debe ser mayor a 0")
	}
	ts, err := txTime(ctx)
	if err != nil {
		return nil, err
	}
	limite := ts.AddDate(0, 0, dias)

	productos, err := sc.ConsultarTodos(ctx)
	if err != nil {
		return nil, err
	}
	resultado := []*Producto{}
	for _, p := range productos {
		if p.Estado == EstadoEntregado || p.FechaVencimiento == "" {
			continue
		}
		vence, err := time.Parse(formatoFecha, p.FechaVencimiento)
		if err != nil {
			continue
		}
		if vence.Before(limite) {
			resultado = append(resultado, p)
		}
	}
	return resultado, nil
}

// =============================================================================
//
//	HELPERS INTERNOS
//
// =============================================================================
func (sc *SupplyContract) existeProducto(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	data, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, err
	}
	return data != nil, nil
}

func (sc *SupplyContract) guardarHistorial(
	ctx contractapi.TransactionContextInterface,
	productoID string, evento EventoHistorial,
) error {
	key, err := ctx.GetStub().CreateCompositeKey("historial", []string{productoID, evento.TxID})
	if err != nil {
		return err
	}
	eJSON, err := json.Marshal(evento)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(key, eJSON)
}

// estaVencido indica si el lote está vencido en la fecha de la transacción.
// Un lote sin fecha (creado con la versión anterior del chaincode) no se bloquea.
func (sc *SupplyContract) estaVencido(p *Producto, ahora time.Time) bool {
	if p.FechaVencimiento == "" {
		return false
	}
	vence, err := time.Parse(formatoFecha, p.FechaVencimiento)
	if err != nil {
		return false
	}
	// vence al final del día indicado
	return !ahora.Before(vence.AddDate(0, 0, 1))
}

// txTime usa la hora de la PROPUESTA (igual en todos los peers endosantes),
// en lugar de time.Now(), que puede diferir entre peers y romper el endorsement.
func txTime(ctx contractapi.TransactionContextInterface) (time.Time, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return time.Time{}, fmt.Errorf("no se pudo leer la hora de la transacción: %v", err)
	}
	return time.Unix(ts.Seconds, int64(ts.Nanos)).UTC(), nil
}

var reNumero = regexp.MustCompile(`-?\d+(?:[.,]\d+)?`)

// parseTemperatura convierte textos como "5°C", "4.5 °C" o "-2" a número.
func parseTemperatura(texto string) (float64, error) {
	m := reNumero.FindString(texto)
	if m == "" {
		return 0, fmt.Errorf("temperatura inválida: %q (use por ejemplo 5°C)", texto)
	}
	return strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64)
}

// =============================================================================
//
//	MAIN
//
// =============================================================================
func main() {
	chaincode, err := contractapi.NewChaincode(&SupplyContract{})
	if err != nil {
		fmt.Printf("Error creando chaincode: %v\n", err)
		return
	}
	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error iniciando chaincode: %v\n", err)
	}
}
