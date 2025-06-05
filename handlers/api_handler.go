package handlers

import (
	"database/sql"
	"fmt"
	"generic-database-service/config"
	"generic-database-service/logger"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type APIHandler struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewAPIHandler(db *gorm.DB, cfg *config.Config) *APIHandler {
	return &APIHandler{DB: db, Cfg: cfg}
}

type TableMetadata struct {
	TableName     string // This will store the qualified name e.g. schema.table
	TableType     string
	TableSchema   string // Schema name itself
	PrimaryKeyCol string
}

// extractSchemaFromDSN (assuming this function is unchanged from previous step)
func extractSchemaFromDSN(dsn string, dbType string) string {
	if dbType == "mysql" {
		parts := strings.Split(dsn, "/")
		if len(parts) > 1 {
			nameAndParams := strings.Split(parts[len(parts)-1], "?")
			return nameAndParams[0]
		}
	}
	if dbType == "postgres" {
		parts := strings.Split(dsn, "/")
		if len(parts) > 1 {
			nameAndParams := strings.Split(parts[len(parts)-1], "?")
			if nameAndParams[0] != "" {
				return nameAndParams[0]
			}
		}
		return "public" // Default to public for PostgreSQL if not in DSN
	}
	if dbType == "sqlserver" {
		if strings.Contains(dsn, "database=") {
			val := strings.Split(dsn, "database=")[1]
			return strings.Split(val, "&")[0]
		}
	}
	if dbType == "oracle" {
        parts := strings.Split(dsn, "/")
        if len(parts) > 0 && !strings.Contains(parts[0], "@") {
            userAndMaybeHost := strings.Split(parts[0], "@")
            return strings.ToUpper(userAndMaybeHost[0])
        }
        if strings.Contains(dsn, "@") {
            userPass := strings.Split(dsn, "@")[0]
            user := strings.Split(userPass, "/")[0]
            if user != "" {
                return strings.ToUpper(user)
            }
        }
    }
	logger.Get().Warnf("Could not reliably determine schema/database name from DSN for type: %s.", dbType)
	return ""
}

func (h *APIHandler) GetTableMetadata(originalTableName string) (*TableMetadata, error) {
	var tableType sql.NullString
	var foundSchema sql.NullString // Schema where the table was found
	var primaryKeyCol sql.NullString

	derivedSchema := extractSchemaFromDSN(h.Cfg.DatabaseConnectionString, h.Cfg.DatabaseType)
	log := logger.Get()

	var query string
	var pkQuery string
	var args []interface{}
	var pkArgs []interface{}

    // This will be the name used for GORM operations, potentially schema-qualified
    tableNameForGorm := originalTableName

	switch h.Cfg.DatabaseType {
	case "mysql":
		// MySQL schema is the database name from DSN. GORM handles this contextually.
		// No prefix needed for tableNameForGorm. derivedSchema is used for INFORMATION_SCHEMA.
		if derivedSchema == "" {
			return nil, fmt.Errorf("MySQL schema (database) could not be determined from DSN for table: %s", originalTableName)
		}
		query = "SELECT T.TABLE_TYPE, T.TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES T WHERE T.TABLE_SCHEMA = ? AND T.TABLE_NAME = ?"
		args = []interface{}{derivedSchema, originalTableName}
		pkQuery = "SELECT K.COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE K JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS C ON K.CONSTRAINT_NAME = C.CONSTRAINT_NAME AND K.TABLE_SCHEMA = C.TABLE_SCHEMA AND K.TABLE_NAME = C.TABLE_NAME WHERE C.CONSTRAINT_TYPE = 'PRIMARY KEY' AND K.TABLE_SCHEMA = ? AND K.TABLE_NAME = ? ORDER BY K.ORDINAL_POSITION LIMIT 1"
		pkArgs = []interface{}{derivedSchema, originalTableName}
        // tableNameForGorm remains originalTableName for MySQL
	case "postgres":
		currentSchema := derivedSchema
		if currentSchema == "" { currentSchema = "public" }
		query = "SELECT t.table_type, t.table_schema FROM information_schema.tables t WHERE t.table_schema = ? AND t.table_name = ?"
		args = []interface{}{currentSchema, originalTableName}
		pkQuery = "SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema AND tc.table_name = kcu.table_name WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = ? AND tc.table_name = ? ORDER BY kcu.ordinal_position LIMIT 1"
		pkArgs = []interface{}{currentSchema, originalTableName}
		tableNameForGorm = fmt.Sprintf("%s.%s", currentSchema, originalTableName)
	case "sqlserver":
		dbName := derivedSchema // Database (catalog) name
		if dbName == "" {
             return nil, fmt.Errorf("SQL Server database name could not be determined for table: %s", originalTableName)
        }
        var actualSchema string = "dbo" // Default schema
        schemaQuery := "SELECT TOP 1 TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_CATALOG = ? AND TABLE_NAME = ?"
        errScan := h.DB.Raw(schemaQuery, dbName, originalTableName).Scan(&actualSchema).Error
        if errScan != nil && errScan != gorm.ErrRecordNotFound {
            log.Warnf("Failed to query actual schema for SQL Server table %s: %v. Defaulting to 'dbo'.", originalTableName, errScan)
        }
        if actualSchema == "" { actualSchema = "dbo" }

		query = "SELECT TABLE_TYPE, TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = ? AND TABLE_CATALOG = ? AND TABLE_SCHEMA = ?"
		args = []interface{}{originalTableName, dbName, actualSchema}
		pkQuery = "SELECT TOP 1 KU.COLUMN_NAME FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS TC INNER JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS KU ON TC.CONSTRAINT_TYPE = 'PRIMARY KEY' AND TC.CONSTRAINT_NAME = KU.CONSTRAINT_NAME AND KU.table_name = TC.table_name WHERE KU.TABLE_CATALOG = ? AND KU.TABLE_SCHEMA = ? AND KU.TABLE_NAME = ? ORDER BY KU.ORDINAL_POSITION"
        pkArgs = []interface{}{dbName, actualSchema, originalTableName}
        tableNameForGorm = fmt.Sprintf("%s.%s", actualSchema, originalTableName)
	case "oracle":
		ownerSchema := derivedSchema
		if ownerSchema == "" {
            return nil, fmt.Errorf("Oracle schema (owner) could not be determined for table: %s", originalTableName)
        }
		oracleQuery := `
            SELECT
                CASE
                    WHEN (SELECT COUNT(*) FROM ALL_TABLES WHERE TABLE_NAME = ? AND OWNER = ?) > 0 THEN 'BASE TABLE'
                    WHEN (SELECT COUNT(*) FROM ALL_VIEWS WHERE VIEW_NAME = ? AND OWNER = ?) > 0 THEN 'VIEW'
                    ELSE NULL
                END AS OBJECT_TYPE,
                ? AS OBJECT_SCHEMA
            FROM DUAL`
		query = oracleQuery
		args = []interface{}{originalTableName, ownerSchema, originalTableName, ownerSchema, ownerSchema}
        pkQuery = "SELECT COLS.COLUMN_NAME FROM ALL_CONSTRAINTS CONS INNER JOIN ALL_CONS_COLUMNS COLS ON CONS.OWNER = COLS.OWNER AND CONS.CONSTRAINT_NAME = COLS.CONSTRAINT_NAME WHERE CONS.CONSTRAINT_TYPE = 'P' AND CONS.OWNER = ? AND CONS.TABLE_NAME = ? AND ROWNUM = 1 ORDER BY COLS.POSITION"
        pkArgs = []interface{}{ownerSchema, originalTableName}
        tableNameForGorm = fmt.Sprintf("%s.%s", ownerSchema, originalTableName)
	default:
		return nil, fmt.Errorf("unsupported database type for metadata query: %s", h.Cfg.DatabaseType)
	}

	log.Debugf("Meta Query: %s, Args: %v", query, args)
	row := h.DB.Raw(query, args...).Row()
	err := row.Scan(&tableType, &foundSchema)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("table/view '%s' not found in derived/default schema '%s'", originalTableName, derivedSchema)
		}
		log.Errorf("DB error fetching metadata for '%s': %v. Query: %s", originalTableName, err, query)
		return nil, fmt.Errorf("DB error for '%s': %w", originalTableName, err)
	}
    if !tableType.Valid || tableType.String == "" {
         return nil, fmt.Errorf("table/view '%s' type unknown/unsupported", originalTableName)
    }

	if pkQuery != "" {
		log.Debugf("PK Query: %s, Args: %v", pkQuery, pkArgs)
		pkRow := h.DB.Raw(pkQuery, pkArgs...).Row()
		errPK := pkRow.Scan(&primaryKeyCol)
		if errPK != nil && errPK != sql.ErrNoRows {
			log.Warnf("Could not determine PK for %s: %v. Query: %s", originalTableName, errPK, pkQuery)
		} else if errPK == sql.ErrNoRows {
			log.Infof("No PK found for %s.", originalTableName)
		}
	}

    schemaToStore := foundSchema.String
    if !foundSchema.Valid || foundSchema.String == "" {
        // If query didn't return schema (e.g. Oracle DUAL query), use derived/default
        if h.Cfg.DatabaseType == "oracle" { schemaToStore = derivedSchema }
        if h.Cfg.DatabaseType == "postgres" && derivedSchema == "" { schemaToStore = "public" }
        if h.Cfg.DatabaseType == "sqlserver" {
            // The actualSchema was determined earlier for SQL Server
            var tempActualSchema string = "dbo"
            schemaQ := "SELECT TOP 1 TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_CATALOG = ? AND TABLE_NAME = ?"
            h.DB.Raw(schemaQ, derivedSchema, originalTableName).Scan(&tempActualSchema)
            if tempActualSchema != "" { schemaToStore = tempActualSchema } else { schemaToStore = "dbo" }
        }
    }


	metadata := &TableMetadata{
		TableName:     tableNameForGorm, // Store qualified name for GORM
		TableType:     strings.ToUpper(tableType.String),
		TableSchema:   schemaToStore, // Actual schema from query or derived
		PrimaryKeyCol: primaryKeyCol.String,
	}
	log.Infof("Metadata for '%s' (GORM Table: %s): Type=%s, Schema=%s, PK=%s", originalTableName, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)
	return metadata, nil
}

func (h *APIHandler) handleGetRequest(c *gin.Context, metadata *TableMetadata) {
	log := logger.Get()
	db := h.DB.Table(metadata.TableName) // metadata.TableName is now qualified

	current, _ := strconv.Atoi(c.DefaultQuery("current", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if current < 1 { current = 1 }
	if pageSize < 1 { pageSize = 10 }
    if pageSize > 100 { pageSize = 100 }
	offset := (current - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	sortParams := c.QueryMap("sort")
	if len(sortParams) > 0 {
		for field, direction := range sortParams {
			orderDirection := "ASC"
			if strings.ToLower(direction) == "descend" { orderDirection = "DESC" }
			col := clause.Column{Name: field}
			db = db.Order(col.Name + " " + orderDirection)
		}
	} else if metadata.PrimaryKeyCol != "" {
		col := clause.Column{Name: metadata.PrimaryKeyCol}
		db = db.Order(col.Name + " DESC")
	}

	queryParams := c.Request.URL.Query()
	var conditions []string
	var values []interface{}

	for key, valArray := range queryParams {
		if key == "current" || key == "pageSize" || strings.HasPrefix(key, "sort[") { continue }
		value := valArray[0]
		colName := key
		opStr := ""
		if strings.Contains(key, "[") && strings.HasSuffix(key, "]") {
			parts := strings.SplitN(key, "[", 2)
			colName = parts[0]
			opStr = strings.TrimSuffix(parts[1], "]")
		}
		quotedCol := clause.Column{Name: colName}.Name
		var condition string
		switch opStr {
		case "", "$eq": condition = fmt.Sprintf("%s = ?", quotedCol); values = append(values, value)
		case "$gt": condition = fmt.Sprintf("%s > ?", quotedCol); values = append(values, value)
		case "$gte": condition = fmt.Sprintf("%s >= ?", quotedCol); values = append(values, value)
		case "$lt": condition = fmt.Sprintf("%s < ?", quotedCol); values = append(values, value)
		case "$lte": condition = fmt.Sprintf("%s <= ?", quotedCol); values = append(values, value)
		case "$ne": condition = fmt.Sprintf("%s <> ?", quotedCol); values = append(values, value)
        case "$like": condition = fmt.Sprintf("%s LIKE ?", quotedCol); values = append(values, value)
		case "$in", "$nin":
			inValues := strings.Split(value, ",")
			if len(inValues) > 0 {
				placeholders := strings.Repeat("?,", len(inValues)-1) + "?"
				op := "IN"; if opStr == "$nin" { op = "NOT IN" }
				condition = fmt.Sprintf("%s %s (%s)", quotedCol, op, placeholders)
				for _, v_ := range inValues { values = append(values, v_) } // Renamed v to v_
			}
		default: log.Warnf("Unsupported op: %s for %s", opStr, colName); continue
		}
		if condition != "" { conditions = append(conditions, condition) }
	}

	if len(conditions) > 0 {
		db = db.Where(strings.Join(conditions, " AND "), values...)
	}

	var results []map[string]interface{}
	if err := db.Find(&results).Error; err != nil {
		log.Errorf("Error fetching data for %s: %v", metadata.TableName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data", "details": err.Error()})
		return
	}

	var totalRecords int64
	countDb := h.DB.Table(metadata.TableName)
	if len(conditions) > 0 {
		countDb = countDb.Where(strings.Join(conditions, " AND "), values...)
	}
	if err := countDb.Count(&totalRecords).Error; err != nil {
		log.Errorf("Error fetching count for %s: %v", metadata.TableName, err)
	}

	log.Infof("Fetched %d records for %s (total: %d, page: %d, pageSize: %d)", len(results), metadata.TableName, totalRecords, current, pageSize)
	c.JSON(http.StatusOK, gin.H{
		"data": results,
		"pagination": gin.H{ "current": current, "pageSize": pageSize, "total": totalRecords },
	})
}

func (h *APIHandler) handlePostRequest(c *gin.Context, metadata *TableMetadata) {
	log := logger.Get()
	var recordData map[string]interface{}
	if err := c.ShouldBindJSON(&recordData); err != nil {
		log.Errorf("Error binding JSON for POST to %s: %v", metadata.TableName, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON", "details": err.Error()})
		return
	}
	log.Debugf("Attempting to create in %s with: %v", metadata.TableName, recordData)
	result := h.DB.Table(metadata.TableName).Create(&recordData) // metadata.TableName is qualified
	if result.Error != nil {
		log.Errorf("Error creating in %s: %v", metadata.TableName, result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create", "details": result.Error.Error()})
		return
	}
	log.Infof("Created in %s. Rows: %d. Data: %v", metadata.TableName, result.RowsAffected, recordData)
	c.JSON(http.StatusCreated, recordData)
}

func (h *APIHandler) handleGetSingleRecord(c *gin.Context, metadata *TableMetadata, id string) {
    log := logger.Get()
    if metadata.PrimaryKeyCol == "" {
        log.Errorf("No primary key defined for table %s, cannot fetch by ID.", metadata.TableName)
        c.JSON(http.StatusBadRequest, gin.H{"error": "No primary key defined for this table."})
        return
    }

    var result map[string]interface{}
    // metadata.TableName is already qualified (e.g. schema.table)
    dbResult := h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).First(&result)

    if dbResult.Error != nil {
        if dbResult.Error == gorm.ErrRecordNotFound {
            log.Warnf("Record not found in %s with ID %s: %v", metadata.TableName, id, dbResult.Error)
            c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
            return
        }
        log.Errorf("Error fetching record from %s with ID %s: %v", metadata.TableName, id, dbResult.Error)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch record", "details": dbResult.Error.Error()})
        return
    }

    log.Infof("Successfully fetched record from %s with ID %s", metadata.TableName, id)
    c.JSON(http.StatusOK, result)
}

func (h *APIHandler) handleUpdateRequest(c *gin.Context, metadata *TableMetadata, id string) {
	log := logger.Get()

	if metadata.PrimaryKeyCol == "" {
		log.Errorf("No primary key defined for table %s, cannot update by ID.", metadata.TableName)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No primary key defined for this table to identify record for update."})
		return
	}

	var recordData map[string]interface{}
	if err := c.ShouldBindJSON(&recordData); err != nil {
		log.Errorf("Error binding JSON for update request to table %s (ID: %s): %v", metadata.TableName, id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON data", "details": err.Error()})
		return
	}
    // Ensure the PK is not part of the update payload if it's different from the ID in path,
    // or handle it according to application rules (e.g. disallow PK change).
    // For simplicity, GORM's Updates won't update PK if it's a standard auto-incrementing GORM model.
    // With map[string]interface{}, it might try if present. It's safer to remove it from map.
    delete(recordData, metadata.PrimaryKeyCol)


	log.Debugf("Attempting to update record in table %s (ID: %s) with data: %v", metadata.TableName, id, recordData)
    // metadata.TableName is already qualified (e.g. schema.table)
	result := h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).Updates(recordData)

	if result.Error != nil {
		log.Errorf("Error updating record in table %s (ID: %s): %v", metadata.TableName, id, result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update record", "details": result.Error.Error()})
		return
	}

	if result.RowsAffected == 0 {
		// Check if record actually exists, could be 0 rows affected if data is same or record not found
        var count int64
        h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).Count(&count)
        if count == 0 {
            log.Warnf("No record found to update in table %s with ID %s.", metadata.TableName, id)
            c.JSON(http.StatusNotFound, gin.H{"error": "Record not found."})
            return
        }
        log.Warnf("Record in table %s with ID %s was not updated (data might be identical or hook prevented update). RowsAffected: 0", metadata.TableName, id)
        // Return 200 with current data or a specific message
	}

	var updatedRecord map[string]interface{}
	fetchResult := h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).First(&updatedRecord)
	if fetchResult.Error != nil {
		log.Errorf("Failed to fetch updated record from %s (ID: %s) after update: %v", metadata.TableName, id, fetchResult.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Record updated, but failed to retrieve the updated data."})
		return
	}

	log.Infof("Successfully updated record in table %s (ID: %s). Rows affected: %d.", metadata.TableName, id, result.RowsAffected)
	c.JSON(http.StatusOK, updatedRecord)
}

func (h *APIHandler) HandleDynamicRequest(c *gin.Context) {
	userFacingTableName := c.Param("tableName") // Original name from path
	log := logger.Get()

	metadata, err := h.GetTableMetadata(userFacingTableName)
	if err != nil {
		log.Errorf("HandleDynamicRequest: Error getting metadata for table %s: %v", userFacingTableName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Infof("HandleDynamicRequest: Routing for %s: %s (GORM Table: %s, Type: %s, Schema: %s, PK: %s)",
		c.Request.Method, userFacingTableName, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)

	isModificationMethod := c.Request.Method == http.MethodPost
	isTable := metadata.TableType == "BASE TABLE" || metadata.TableType == "TABLE"

	if isModificationMethod && !isTable {
		log.Warnf("HandleDynamicRequest: %s attempt on non-table type: %s for %s", c.Request.Method, metadata.TableType, userFacingTableName)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": fmt.Sprintf("%s operation only allowed on tables, not %s.", c.Request.Method, metadata.TableType)})
		return
	}

	switch c.Request.Method {
	case http.MethodGet:
		h.handleGetRequest(c, metadata)
	case http.MethodPost:
		h.handlePostRequest(c, metadata)
	default:
		log.Warnf("HandleDynamicRequest: Unsupported method %s for %s", c.Request.Method, userFacingTableName)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed for this resource path."})
	}
}

func (h *APIHandler) HandleDynamicRequestWithID(c *gin.Context) {
	userFacingTableName := c.Param("tableName")
	id := c.Param("id")
	log := logger.Get()

	metadata, err := h.GetTableMetadata(userFacingTableName)
	if err != nil {
		log.Errorf("HandleDynamicRequestWithID: Error getting metadata for table %s: %v", userFacingTableName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Infof("HandleDynamicRequestWithID: Routing for %s: %s/%s (GORM Table: %s, Type: %s, Schema: %s, PK: %s)",
		c.Request.Method, userFacingTableName, id, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)

	isModificationMethod := c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch || c.Request.Method == http.MethodDelete
	isTable := metadata.TableType == "BASE TABLE" || metadata.TableType == "TABLE"

	if isModificationMethod && !isTable {
		log.Warnf("HandleDynamicRequestWithID: %s attempt on non-table type: %s for %s/%s", c.Request.Method, metadata.TableType, userFacingTableName, id)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": fmt.Sprintf("%s operation only allowed on tables, not %s.", c.Request.Method, metadata.TableType)})
		return
	}

	switch c.Request.Method {
	case http.MethodGet:
        h.handleGetSingleRecord(c, metadata, id)
	case http.MethodPut, http.MethodPatch:
		h.handleUpdateRequest(c, metadata, id)
	case http.MethodDelete:
		c.JSON(http.StatusNotImplemented, gin.H{"message": "DELETE operation not yet implemented."})
	default:
		log.Warnf("HandleDynamicRequestWithID: Unsupported method %s for %s/%s", c.Request.Method, userFacingTableName, id)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed for this resource path."})
	}
}
