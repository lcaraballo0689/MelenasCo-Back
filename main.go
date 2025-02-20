package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq" // Driver de PostgreSQL
	"gopkg.in/yaml.v3"
)

// Estructura para los datos del certificado
type CertificateData struct {
	NombreCliente        string `json:"nombre_cliente"`
	ApellidoCliente      string `json:"apellido_cliente"`
	EmailCliente         string `json:"email_cliente"`
	NombreProducto       string `json:"nombre_producto"`
	DescripcionProducto  string `json:"descripcion_producto"`
	TipoCabello          string `json:"tipo_cabello"`
	Color                string `json:"color"`
	Longitud             string `json:"longitud"`
	ImagenURL            string `json:"imagen_url"`
	FechaCompra          string `json:"fecha_compra"`
	FechaEmision         string `json:"fecha_emision"`
	NumeroCertificado    string `json:"numero_certificado"`
	EstadoPago           string `json:"estado_pago"`
}

// Estructura para los datos del producto
type Product struct {
	ID          int    `json:"id"`
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
	TipoCabello string `json:"tipo_cabello"`
	Color       string `json:"color"`
	Longitud    string `json:"longitud"`
	ImagenURL   string `json:"imagen_url"`
}

// Configuración leída desde el archivo YAML
type Config struct {
	DB struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		DBName   string `yaml:"dbname"`
	} `yaml:"db"`
	API struct {
		XSecret string `yaml:"x_secret"`
		XAPIKey string `yaml:"x_api_key"`
	} `yaml:"rocketfy"`
}

var config Config

func main() {
	// Cargar la configuración desde el archivo YAML
	err := loadConfig("config.yml")
	if err != nil {
		log.Fatal("Error al cargar la configuración:", err)
	}

	// Configura el manejador del endpoint
	http.HandleFunc("/obtener_certificado", obtenerCertificadoHandler)
	http.HandleFunc("/obtener_productos", obtenerProductosHandler)

	// Inicia el servidor en el puerto 8080 (o el que prefieras)
	log.Println("Servidor iniciado en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Función para obtener productos desde la API externa
func obtenerProductos() ([]map[string]interface{}, error) {
	// Hacer la solicitud GET a la API externa
	req, err := http.NewRequest("GET", "https://ms-public-api.rocketfy.com/rocketfy/api/v1/products", nil)
	if err != nil {
		return nil, fmt.Errorf("Error al crear la solicitud: %v", err)
	}

	// Configuración de los headers usando datos del config.yml
	req.Header.Set("accept", "application/json")
	req.Header.Set("x-secret", config.API.XSecret)
	req.Header.Set("x-api-key", config.API.XAPIKey)

	// Ejecutar la solicitud
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error al hacer la solicitud: %v", err)
	}
	defer resp.Body.Close()

	// Leer la respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error al leer la respuesta: %v", err)
	}

	// Verificar que la respuesta sea exitosa (200 OK)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Error en la solicitud, código de estado: %d", resp.StatusCode)
	}

	// Deserializar los datos JSON en una estructura genérica (map)
	var result []map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("Error al deserializar los datos: %v", err)
	}

	return result, nil
}

// Handler para el endpoint que devuelve los productos
func obtenerProductosHandler(w http.ResponseWriter, r *http.Request) {
	// Permitir solicitudes desde cualquier origen
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")

	// Obtener los productos desde la API externa
	products, err := obtenerProductos()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al obtener productos: %v", err), http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Convertir los productos a JSON y enviarlos como respuesta
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(products)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al convertir productos a JSON: %v", err), http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

func obtenerCertificadoHandler(w http.ResponseWriter, r *http.Request) {
	// Permitir solicitudes desde cualquier origen (ajusta según sea necesario)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET") // Ajusta los métodos permitidos
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type") // Ajusta los encabezados permitidos

	// Si es una solicitud OPTIONS (preflight), responder sin procesar
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Obtener el número de certificado de la query string
	numeroCertificado := r.URL.Query().Get("certificateNumber")
	if numeroCertificado == "" {
		http.Error(w, "Número de certificado requerido", http.StatusBadRequest)
		return
	}

	// Conectar a la base de datos
	db, err := conectarDB()
	if err != nil {
		http.Error(w, "Error al conectar a la base de datos", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer db.Close()

	// Consultar la base de datos
	data, err := consultarCertificado(db, numeroCertificado)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Certificado no encontrado", http.StatusNotFound)
		} else {
			http.Error(w, "Error al consultar la base de datos", http.StatusInternalServerError)
		}
		log.Println(err)
		return
	}

	// Convertir a JSON y enviar la respuesta
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func conectarDB() (*sql.DB, error) {
	// Construir la cadena de conexión
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.DB.Host, config.DB.Port, config.DB.User, config.DB.Password, config.DB.DBName)

	// Conectar a la base de datos
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	// Verificar la conexión
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func consultarCertificado(db *sql.DB, numeroCertificado string) (*CertificateData, error) {
	// Consulta SQL
	sqlStatement := `
		SELECT
			c.nombre AS nombre_cliente,
			c.apellido AS apellido_cliente,
			c.email AS email_cliente,
			p.nombre AS nombre_producto,
			p.descripcion AS descripcion_producto,
			p.tipo_cabello,
			p.color,
			p.longitud,
			p.imagen_url,
			com.fecha_compra,
			cer.fecha_emision,
			cer.numero_certificado,
			com.estado_pago
		FROM Certificados cer
		JOIN Compras com ON cer.certificado_id = com.certificado_id
		JOIN Clientes c ON com.cliente_id = c.cliente_id
		JOIN DetallesCompra dc ON com.compra_id = dc.compra_id
		JOIN Productos p ON dc.producto_id = p.producto_id
		WHERE cer.numero_certificado = $1`

	// Ejecutar la consulta
	row := db.QueryRow(sqlStatement, numeroCertificado)

	// Escanear los resultados en la estructura CertificateData
	var data CertificateData
	err := row.Scan(
		&data.NombreCliente,
		&data.ApellidoCliente,
		&data.EmailCliente,
		&data.NombreProducto,
		&data.DescripcionProducto,
		&data.TipoCabello,
		&data.Color,
		&data.Longitud,
		&data.ImagenURL,
		&data.FechaCompra,
		&data.FechaEmision,
		&data.NumeroCertificado,
		&data.EstadoPago,
	)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

// Cargar la configuración desde el archivo YAML
func loadConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	return nil
}
