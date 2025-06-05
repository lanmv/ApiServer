# Generic Database API Service (Golang)

## 1. Project Overview

This project provides a generic database interface service developed in Golang. It allows users to configure database parameters via a `config.json` file and then automatically exposes CRUD (Create, Read, Update, Delete) API endpoints for the tables and views within that database, as well as endpoints to list available tables and views.

The primary goal is to offer a quick and flexible way to make database content accessible via a RESTful API without writing custom boilerplate code for each table.

## 2. Features

*   **Dynamic CRUD Operations:** Provides RESTful endpoints for tables (Create, Read, Update, Delete) and views (Read-only).
*   **Metadata Endpoints:** Provides endpoints to list all available tables (`/api/v1/getTables`) and views (`/api/v1/getViews`).
*   **Health Check:** A public `/health` endpoint for monitoring service status.
*   **Multiple Database Support:**
    *   Currently Supported: MySQL, PostgreSQL, SQL Server.
    *   Planned: Oracle (support is partially implemented but pending robust driver/dialect testing).
*   **Configuration Driven:** Database connection, service port, API keys, and log levels are all managed via `config.json`.
*   **Advanced Querying (for CRUD on tables/views):**
    *   **Filtering:** Supports simple equality filters (e.g., `age=30`) and complex operator-based filters (e.g., `age[\$gt]=18`, `status[\$in]=active,pending`).
    *   **Sorting:** Allows sorting results by one or more fields in ascending or descending order (e.g., `sort[name]=ascend`). Defaults to primary key descending.
    *   **Pagination:** Supports `current` page and `pageSize` parameters for paginating results.
*   **Structured Logging:** Detailed logging of requests and responses to daily rotated log files in the `logs/` directory.
*   **Dynamic Schema Discovery:** Automatically determines table/view metadata, including primary keys.
*   **Optional API Key Authentication:** Protects data endpoints while allowing public access to health checks.


## 3. Setup and Installation

### Prerequisites

*   **Go:** Version 1.21 or higher is recommended (the project was developed with 1.23).

### Installation

1.  **Clone the repository (or download the source code):**
    ```bash
    git clone <repository_url>
    cd generic-database-service
    ```

2.  **Install dependencies:**
    If you have Go modules enabled (which is default), dependencies are typically handled automatically when you build or run. You can explicitly tidy them:
    ```bash
    go mod tidy
    ```
    Or, to ensure all dependencies from `go.mod` are fetched:
    ```bash
    go get ./...
    ```

## 4. Configuration (`config.json`)

Create a `config.json` file in the root of the project directory. This file controls the service's behavior.

### Configuration Fields:

*   `database_type` (string): Specifies the type of database.
    *   Supported: `"mysql"`, `"postgres"`, `"sqlserver"`.
*   `database_connection_string` (string): The DSN (Data Source Name) for connecting to your database. **See examples below.**
*   `service_port` (string): The port on which the API service will listen (e.g., `"8080"`).
*   `api_secret_key` (string): A secret key for API authentication for `/api/v1/*` routes. If empty or not set, authentication for these routes is disabled. Example: `"your-super-secret-key"`.
*   `log_level` (string): The logging level for the application.
    *   Supported: `"debug"`, `"info"`, `"warn"`, `"error"`.

### Example `config.json`:

```json
{
  "database_type": "mysql",
  "database_connection_string": "user:password@tcp(127.0.0.1:3306)/mydatabase?charset=utf8mb4&parseTime=True&loc=Local",
  "service_port": "8080",
  "api_secret_key": "replace-with-your-actual-secret-key",
  "log_level": "info"
}
```

### Database Connection String Examples:

*   **MySQL:**
    ```
    username:password@tcp(hostname:port)/database_name?charset=utf8mb4&parseTime=True&loc=Local
    ```
    Example: `root:password123@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True&loc=Local`

*   **PostgreSQL:**
    Uses a DSN string or a URL format.
    ```
    host=your_host port=your_port user=your_user password=your_password dbname=your_dbname sslmode=disable TimeZone=Asia/Shanghai
    ```
    or URL format:
    ```
    postgresql://your_user:your_password@your_host:your_port/your_dbname?sslmode=disable
    ```
    Example: `host=localhost port=5432 user=pguser password=pgpass dbname=appdb sslmode=disable`

*   **SQL Server:**
    Uses a URL format.
    ```
    sqlserver://username:password@hostname:port?database=database_name&encrypt=disable
    ```
    (Note: `encrypt=disable` might be needed for local/test environments. For production, consider `encrypt=true` and trust server certificate settings.)
    Example: `sqlserver://sa:YourStrong!Password@127.0.0.1:1433?database=masterdb&encrypt=disable`

## 5. Running the Service

Navigate to the project's root directory and run:

```bash
go run main.go
```

The service will start, and you should see log output indicating it's listening on the configured port (e.g., `Starting server on :8080`).

## 6. API Endpoints and Usage

The service exposes the following endpoints.
The base URL for the main API is `http://localhost:<service_port>/api/v1`. Replace `<service_port>` with the port configured in `config.json`.
Replace `{TableName}` with the actual name of your database table or view.
Replace `{id}` with the actual primary key value of a record.

---

### 6.1 Health Check

*   **Endpoint:** `GET /health`
*   **Description:** Provides a simple health check for the service.
*   **Authentication:** None required. This endpoint is public.
*   **Example Request:** `GET /health`
*   **Example Success Response (200 OK):**
    ```json
    {
      "status": "UP",
      "timestamp": "2023-10-29T12:00:00.000Z"
    }
    ```

---

### 6.2 List All Tables

*   **Endpoint:** `GET /api/v1/getTables`
*   **Description:** Retrieves a list of all accessible table names in the configured database/schema.
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Example Request:** `GET /api/v1/getTables`
*   **Example Success Response (200 OK):**
    ```json
    {
      "tables": [
        "users",
        "products",
        "orders_archive"
      ]
    }
    ```

---

### 6.3 List All Views

*   **Endpoint:** `GET /api/v1/getViews`
*   **Description:** Retrieves a list of all accessible view names in the configured database/schema.
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Example Request:** `GET /api/v1/getViews`
*   **Example Success Response (200 OK):**
    ```json
    {
      "views": [
        "active_customers",
        "product_summary"
      ]
    }
    ```

---

### 6.4 List Records from Table/View

*   **Endpoint:** `GET /api/v1/{TableName}`
*   **Description:** Retrieves a list of records from the specified table or view. Supports filtering, sorting, and pagination.
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Query Parameters:**
    *   **Filtering (Simple Mode):** `?fieldName=value`
        *   Example: `/api/v1/users?status=active&department_id=5`
    *   **Filtering (Complex Mode):** `?fieldName[$operator]=value`
        *   Supported operators: `$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`, `$like`, `$in`, `$nin`.
        *   Example: `/api/v1/products?price[\$gte]=50&category[\$in]=electronics,books`
    *   **Sorting:** `?sort[fieldName]=ascend|descend`
        *   Example: `/api/v1/orders?sort[order_date]=descend&sort[total_amount]=ascend`
        *   If not provided, defaults to primary key descending for tables.
    *   **Pagination:** `current` (default: 1), `pageSize` (default: 10, max: 100).
        *   Example: `/api/v1/logs?current=3&pageSize=20`
*   **Example Success Response (200 OK):**
    ```json
    {
      "data": [
        { "id": 1, "name": "Product A", "price": 25.50 },
        { "id": 2, "name": "Product B", "price": 75.00 }
      ],
      "pagination": {
        "current": 1,
        "pageSize": 10,
        "total": 2
      }
    }
    ```

---

### 6.5 Get Record by ID from Table/View

*   **Endpoint:** `GET /api/v1/{TableName}/{id}`
*   **Description:** Retrieves a single record by its primary key from the specified table or view.
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Example Request:** `GET /api/v1/users/123`
*   **Example Success Response (200 OK):**
    ```json
    {
      "id": 123,
      "username": "john_doe",
      "email": "john.doe@example.com"
    }
    ```

---

### 6.6 Create Record in Table

*   **Endpoint:** `POST /api/v1/{TableName}`
*   **Description:** Creates a new record in the specified table. (Not applicable to views).
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Request Body:** A JSON object representing the record to create.
*   **Example Success Response (201 Created):** Includes the created record with DB-generated fields.
    ```json
    {
      "id": 101,
      "name": "New Gadget",
      // ... other fields
    }
    ```

---

### 6.7 Update Record in Table

*   **Endpoint:** `PUT /api/v1/{TableName}/{id}` or `PATCH /api/v1/{TableName}/{id}`
*   **Description:** Updates an existing record by its primary key in the specified table. (Not applicable to views).
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Request Body:** A JSON object containing the fields to update.
*   **Example Success Response (200 OK):** Includes the full updated record.
    ```json
    {
      "id": 101,
      "name": "Updated Gadget",
      "price": 189.99,
      // ... other fields
    }
    ```

---

### 6.8 Delete Record from Table

*   **Endpoint:** `DELETE /api/v1/{TableName}/{id}`
*   **Description:** Deletes a record by its primary key from the specified table. (Not applicable to views).
*   **Authentication:** Requires API Key if `api_secret_key` is configured.
*   **Example Success Response:** `204 No Content` (with an empty body).

## 7. Logging

*   Logs are stored in the `./logs/` directory relative to the project root.
*   Log files are named by date (e.g., `2023-10-28.log`).
*   The log level can be configured in `config.json` (`debug`, `info`, `warn`, `error`).
*   The logging middleware captures details for each request, including method, path, status, latency, client IP, user agent, and a snippet of the response body for errors.

## 8. Supported Databases

*   **Currently Implemented & Tested:**
    *   MySQL
    *   PostgreSQL
    *   SQL Server
*   **Planned / In Progress:**
    *   Oracle (Initial GORM dialect integration faced challenges in the development environment; requires further testing and potentially a different driver/dialect combination).

## 9. License

This project is licensed under the MIT License.

```
MIT License

Copyright (c) [Year] [Your Name/Organization]
# Replace [Year] and [Your Name/Organization] appropriately

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
