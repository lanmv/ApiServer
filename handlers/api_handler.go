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
	TableName     string // This will be schema-qualified for non-MySQL DBs if schema is found
	TableType     string // E.g., 'BASE TABLE', 'VIEW', 'TABLE'
	TableSchema   string // The actual schema name found
	PrimaryKeyCol string // Name of the primary key column
}

// extractSchemaFromDSN (Copied from previous version)
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
		return "public"
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

// GetTableMetadata (Copied and refined from previous version)
func (h *APIHandler) GetTableMetadata(tableName string) (*TableMetadata, error) {
	var tableType sql.NullString
	var tableSchemaVal sql.NullString // Renamed to avoid conflict with local var 'tableSchema'
	var primaryKeyCol sql.NullString

	originalTableName := tableName // Keep original name for user-facing messages if needed
	derivedSchema := extractSchemaFromDSN(h.Cfg.DatabaseConnectionString, h.Cfg.DatabaseType)
	log := logger.Get()

	var query string
	var pkQuery string
	var args []interface{}
	var pkArgs []interface{}

	currentSchemaForQuery := derivedSchema // This will be used in queries

	switch h.Cfg.DatabaseType {
	case "mysql":
		if currentSchemaForQuery == "" {
			return nil, fmt.Errorf("MySQL schema (database name) could not be determined from DSN for table: %s", originalTableName)
		}
		query = "SELECT T.TABLE_TYPE, T.TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES T WHERE T.TABLE_SCHEMA = ? AND T.TABLE_NAME = ?"
		args = []interface{}{currentSchemaForQuery, originalTableName}
		pkQuery = "SELECT K.COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE K JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS C ON K.CONSTRAINT_NAME = C.CONSTRAINT_NAME AND K.TABLE_SCHEMA = C.TABLE_SCHEMA AND K.TABLE_NAME = C.TABLE_NAME WHERE C.CONSTRAINT_TYPE = 'PRIMARY KEY' AND K.TABLE_SCHEMA = ? AND K.TABLE_NAME = ? ORDER BY K.ORDINAL_POSITION LIMIT 1"
		pkArgs = []interface{}{currentSchemaForQuery, originalTableName}
	case "postgres":
		if currentSchemaForQuery == "" { currentSchemaForQuery = "public" }
		query = "SELECT t.table_type, t.table_schema FROM information_schema.tables t WHERE t.table_schema = ? AND t.table_name = ?"
		args = []interface{}{currentSchemaForQuery, originalTableName}
		pkQuery = "SELECT kcu.column_name FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema AND tc.table_name = kcu.table_name WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = ? AND tc.table_name = ? ORDER BY kcu.ordinal_position LIMIT 1"
		pkArgs = []interface{}{currentSchemaForQuery, originalTableName}
	case "sqlserver":
		dbName := currentSchemaForQuery // For SQL Server, derivedSchema is the database name (catalog)
		if dbName == "" {
             return nil, fmt.Errorf("SQL Server database name could not be determined from DSN for table: %s", originalTableName)
        }
        // Determine the actual schema (like 'dbo') for the table
        var actualSchemaForTable string = "dbo" // Default
        schemaQuery := "SELECT TOP 1 TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_CATALOG = ? AND TABLE_NAME = ?"
        errSchema := h.DB.Raw(schemaQuery, dbName, originalTableName).Scan(&actualSchemaForTable).Error
        if errSchema != nil && errSchema != gorm.ErrRecordNotFound { // Check for actual error, not just no rows
             log.Warnf("Failed to query actual schema for SQL Server table %s in DB %s, defaulting to 'dbo'. Error: %v", originalTableName, dbName, errSchema)
        }
        if actualSchemaForTable == "" {actualSchemaForTable = "dbo"} // Ensure it's not empty if scan resulted in empty string
        currentSchemaForQuery = actualSchemaForTable // This is the schema like 'dbo'

		query = "SELECT TABLE_TYPE, TABLE_SCHEMA FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = ? AND TABLE_CATALOG = ? AND TABLE_SCHEMA = ?"
		args = []interface{}{originalTableName, dbName, currentSchemaForQuery}
		pkQuery = "SELECT TOP 1 KU.COLUMN_NAME FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS TC INNER JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS KU ON TC.CONSTRAINT_TYPE = 'PRIMARY KEY' AND TC.CONSTRAINT_NAME = KU.CONSTRAINT_NAME AND KU.table_name = TC.table_name WHERE KU.TABLE_CATALOG = ? AND KU.TABLE_SCHEMA = ? AND KU.TABLE_NAME = ? ORDER BY KU.ORDINAL_POSITION"
        pkArgs = []interface{}{dbName, currentSchemaForQuery, originalTableName}
	case "oracle":
		ownerSchema := currentSchemaForQuery
		if ownerSchema == "" {
            return nil, fmt.Errorf("Oracle schema (owner) could not be determined from DSN for table: %s", originalTableName)
        }
		query = `
            SELECT
                CASE
                    WHEN (SELECT COUNT(*) FROM ALL_TABLES WHERE TABLE_NAME = :1 AND OWNER = :2) > 0 THEN 'BASE TABLE'
                    WHEN (SELECT COUNT(*) FROM ALL_VIEWS WHERE VIEW_NAME = :1 AND OWNER = :2) > 0 THEN 'VIEW'
                    ELSE NULL
                END AS OBJECT_TYPE,
                :2 AS OBJECT_SCHEMA
            FROM DUAL`
        query = strings.ReplaceAll(strings.ReplaceAll(query, ":1", "?"), ":2", "?")
		args = []interface{}{originalTableName, ownerSchema, originalTableName, ownerSchema, ownerSchema}

        pkQuery = "SELECT COLS.COLUMN_NAME FROM ALL_CONSTRAINTS CONS INNER JOIN ALL_CONS_COLUMNS COLS ON CONS.OWNER = COLS.OWNER AND CONS.CONSTRAINT_NAME = COLS.CONSTRAINT_NAME WHERE CONS.CONSTRAINT_TYPE = 'P' AND CONS.OWNER = ? AND CONS.TABLE_NAME = ? AND ROWNUM = 1 ORDER BY COLS.POSITION"
        pkArgs = []interface{}{ownerSchema, originalTableName}
	default:
		return nil, fmt.Errorf("unsupported database type for metadata query: %s", h.Cfg.DatabaseType)
	}

	log.Debugf("Meta Query for %s: %s, Args: %v", originalTableName, query, args)
	row := h.DB.Raw(query, args...).Row()
	err := row.Scan(&tableType, &tableSchemaVal)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("table/view '%s' not found in schema '%s' (or default)", originalTableName, currentSchemaForQuery)
		}
		log.Errorf("DB error fetching metadata for '%s': %v. Query: %s", originalTableName, err, query)
		return nil, fmt.Errorf("database error for '%s': %w", originalTableName, err)
	}
    if !tableType.Valid || tableType.String == "" {
         return nil, fmt.Errorf("table/view '%s' type unknown/unsupported", originalTableName)
    }

	if pkQuery != "" {
		log.Debugf("PK Query for %s: %s, Args: %v", originalTableName, pkQuery, pkArgs)
		pkRow := h.DB.Raw(pkQuery, pkArgs...).Row()
		errPK := pkRow.Scan(&primaryKeyCol)
		if errPK != nil && errPK != sql.ErrNoRows {
			log.Warnf("Could not determine PK for %s: %v. Query: %s", originalTableName, errPK, pkQuery)
		} else if errPK == sql.ErrNoRows {
			log.Infof("No PK found for %s.", originalTableName)
		}
	}

    qualifiedTableNameForGorm := originalTableName
    finalSchema := currentSchemaForQuery // Default to derived/default schema
    if tableSchemaVal.Valid && tableSchemaVal.String != "" {
        finalSchema = tableSchemaVal.String // Use schema found by query if valid
    }

    if finalSchema != "" && h.Cfg.DatabaseType != "mysql" {
        qualifiedTableNameForGorm = finalSchema + "." + originalTableName
    }


	metadata := &TableMetadata{
		TableName:     qualifiedTableNameForGorm,
		TableType:     strings.ToUpper(tableType.String),
		TableSchema:   finalSchema,
		PrimaryKeyCol: primaryKeyCol.String,
	}
	log.Infof("Metadata for '%s' (GORM uses: %s): Type=%s, Schema=%s, PK=%s", originalTableName, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)
	return metadata, nil
}


// handleGetRequest (Copied from previous version)
func (h *APIHandler) handleGetRequest(c *gin.Context, metadata *TableMetadata) {
	log := logger.Get()
	db := h.DB.Table(metadata.TableName)


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
		quotedCol := clause.Column{Name: colName}.Name // GORM handles actual quoting
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
				for _, v_ := range inValues { values = append(values, v_) }
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

// handlePostRequest (Copied from previous version)
func (h *APIHandler) handlePostRequest(c *gin.Context, metadata *TableMetadata) {
	log := logger.Get()
	var recordData map[string]interface{}
	if err := c.ShouldBindJSON(&recordData); err != nil {
		log.Errorf("Error binding JSON for POST to %s: %v", metadata.TableName, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON", "details": err.Error()})
		return
	}
	log.Debugf("Attempting to create in %s with: %v", metadata.TableName, recordData)
	result := h.DB.Table(metadata.TableName).Create(&recordData)
	if result.Error != nil {
		log.Errorf("Error creating in %s: %v", metadata.TableName, result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create", "details": result.Error.Error()})
		return
	}
	log.Infof("Created in %s. Rows: %d. Data: %v", metadata.TableName, result.RowsAffected, recordData)
	c.JSON(http.StatusCreated, recordData)
}

// handleGetSingleRecord (Copied from previous version)
func (h *APIHandler) handleGetSingleRecord(c *gin.Context, metadata *TableMetadata, id string) {
    log := logger.Get()
    if metadata.PrimaryKeyCol == "" {
        log.Errorf("No primary key defined for table %s, cannot fetch by ID.", metadata.TableName)
        c.JSON(http.StatusBadRequest, gin.H{"error": "No primary key defined for this table."})
        return
    }

    var result map[string]interface{}
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

// handleUpdateRequest (Copied from previous version, with slight refinement for 404 on 0 rows affected)
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
    delete(recordData, metadata.PrimaryKeyCol)

	log.Debugf("Attempting to update record in table %s (ID: %s) with data: %v", metadata.TableName, id, recordData)
	result := h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).Updates(recordData)

	if result.Error != nil {
		log.Errorf("Error updating record in table %s (ID: %s): %v", metadata.TableName, id, result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update record", "details": result.Error.Error()})
		return
	}

	if result.RowsAffected == 0 {
		var count int64
		h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).Count(&count)
		if count == 0 {
			log.Warnf("No record found to update in table %s with ID %s.", metadata.TableName, id)
			c.JSON(http.StatusNotFound, gin.H{"error": "Record not found."})
		} else {
			log.Infof("Record in table %s with ID %s was not updated (data may be identical or hooks prevented update). Rows affected: 0", metadata.TableName, id)
			var existingRecord map[string]interface{}
			h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).First(&existingRecord)
			c.JSON(http.StatusOK, existingRecord)
		}
		return
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

// handleDeleteRequest implements logic for DELETE to remove a record.
func (h *APIHandler) handleDeleteRequest(c *gin.Context, metadata *TableMetadata, id string) {
	log := logger.Get()

	if metadata.PrimaryKeyCol == "" {
		log.Errorf("No primary key defined for table %s, cannot delete by ID.", metadata.TableName)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No primary key defined for this table to identify record for deletion."})
		return
	}

	log.Debugf("Attempting to delete record in table %s (ID: %s)", metadata.TableName, id)

	// GORM's Delete method requires a pointer to a struct or map.
	// For dynamic tables, an empty map is suitable when using .Table() and .Where().
	// The actual type of the argument to Delete() doesn't matter much here as long as it's a pointer.
	result := h.DB.Table(metadata.TableName).Where(fmt.Sprintf("%s = ?", metadata.PrimaryKeyCol), id).Delete(&map[string]interface{}{})

	if result.Error != nil {
		log.Errorf("Error deleting record in table %s (ID: %s): %v", metadata.TableName, id, result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete record", "details": result.Error.Error()})
		return
	}

	if result.RowsAffected == 0 {
		log.Warnf("No record found to delete in table %s with ID %s.", metadata.TableName, id)
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found."})
		return
	}

	log.Infof("Successfully deleted record in table %s (ID: %s). Rows affected: %d.", metadata.TableName, id, result.RowsAffected)
	c.Status(http.StatusNoContent)
}


// HandleDynamicRequest routes requests for /api/v1/:tableName (GET list, POST create)
func (h *APIHandler) HandleDynamicRequest(c *gin.Context) {
	tableName := c.Param("tableName")
	log := logger.Get()

	metadata, err := h.GetTableMetadata(tableName)
	if err != nil {
		log.Errorf("Error getting metadata for table %s: %v", tableName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Routing request for %s: %s (GORM uses: %s, Type: %s, Schema: %s, PK: %s)",
		c.Request.Method, tableName, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)

	isModificationMethod := c.Request.Method == http.MethodPost
	isTable := metadata.TableType == "BASE TABLE" || metadata.TableType == "TABLE"

	if isModificationMethod && !isTable {
		log.Warnf("%s attempt on non-table type: %s for %s", c.Request.Method, metadata.TableType, tableName)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": fmt.Sprintf("%s operation only allowed on tables, not %s.", c.Request.Method, metadata.TableType)})
		return
	}

	switch c.Request.Method {
	case http.MethodGet:
		h.handleGetRequest(c, metadata)
	case http.MethodPost:
		h.handlePostRequest(c, metadata)
	default:
		log.Warnf("Unsupported method %s routed to HandleDynamicRequest for %s", c.Request.Method, tableName)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed for this resource path."})
	}
}

// HandleDynamicRequestWithID routes requests for /api/v1/:tableName/:id
func (h *APIHandler) HandleDynamicRequestWithID(c *gin.Context) {
	tableName := c.Param("tableName")
	id := c.Param("id")
	log := logger.Get()

	metadata, err := h.GetTableMetadata(tableName)
	if err != nil {
		log.Errorf("Error getting metadata for table %s: %v", tableName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Routing ID-based request for %s: %s/%s (GORM uses: %s, Type: %s, Schema: %s, PK: %s)",
		c.Request.Method, tableName, id, metadata.TableName, metadata.TableType, metadata.TableSchema, metadata.PrimaryKeyCol)

	isModificationMethod := c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch || c.Request.Method == http.MethodDelete
	isTable := metadata.TableType == "BASE TABLE" || metadata.TableType == "TABLE"

	if isModificationMethod && !isTable {
		log.Warnf("%s attempt on non-table type: %s for %s/%s", c.Request.Method, metadata.TableType, tableName, id)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": fmt.Sprintf("%s operation only allowed on tables, not %s.", c.Request.Method, metadata.TableType)})
		return
	}

	switch c.Request.Method {
	case http.MethodGet:
        h.handleGetSingleRecord(c, metadata, id)
	case http.MethodPut, http.MethodPatch:
		h.handleUpdateRequest(c, metadata, id)
	case http.MethodDelete:
		h.handleDeleteRequest(c, metadata, id)
	default:
		log.Warnf("Unsupported method %s routed to HandleDynamicRequestWithID for %s/%s", c.Request.Method, tableName, id)
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed for this resource path."})
	}
}
