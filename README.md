# Generic Database API Service (Golang)

## 1. Project Overview

This project provides a generic database interface service developed in Golang. It allows users to configure database parameters via a `config.json` file and then automatically exposes CRUD (Create, Read, Update, Delete) API endpoints for the tables and views within that database.

The primary goal is to offer a quick and flexible way to make database content accessible via a RESTful API without writing custom boilerplate code for each table.

## 2. Features

*   **Dynamic CRUD Operations:** Provides RESTful endpoints for tables (Create, Read, Update, Delete) and views (Read-only).
*   **Multiple Database Support:**
    *   Currently Supported: MySQL, PostgreSQL, SQL Server.
    *   Planned: Oracle (support is partially implemented but pending robust driver/dialect testing).
*   **Configuration Driven:** Database connection, service port, API keys, and log levels are all managed via `config.json`.
*   **Advanced Querying:**
    *   **Filtering:** Supports simple equality filters (e.g., `age=30`) and complex operator-based filters (e.g., `age[\$gt]=18`, `status[\$in]=active,pending`).
    *   **Sorting:** Allows sorting results by one or more fields in ascending or descending order (e.g., `sort[name]=ascend`). Defaults to primary key descending.
    *   **Pagination:** Supports `current` page and `pageSize` parameters for paginating results.
*   **Structured Logging:** Detailed logging of requests and responses to daily rotated log files in the `logs/` directory.
*   **Dynamic Schema Discovery:** Automatically determines table/view metadata, including primary keys.

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
*   `api_secret_key` (string): A secret key for API authentication (feature planned for future middleware). For now, it can be any string. Example: `"your-super-secret-key"`.
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

The base URL for the API is `http://localhost:<service_port>/api/v1`. Replace `<service_port>` with the port configured in `config.json`.
Replace `{TableName}` with the actual name of your database table or view.
Replace `{id}` with the actual primary key value of a record.

---

### 6.1 List Records

*   **Endpoint:** `GET /{TableName}`
*   **Description:** Retrieves a list of records from the specified table or view. Supports filtering, sorting, and pagination.
*   **Query Parameters:**
    *   **Filtering (Simple Mode):** `?fieldName=value`
        *   Example: `/api/v1/users?status=active&department_id=5`
    *   **Filtering (Complex Mode):** `?fieldName[$operator]=value`
        *   Supported operators:
            *   `$eq`: Equals (e.g., `age[\$eq]=30`)
            *   `$ne`: Not Equals (e.g., `status[\$ne]=archived`)
            *   `$gt`: Greater Than (e.g., `price[\$gt]=100`)
            *   `$gte`: Greater Than or Equal To (e.g., `quantity[\$gte]=10`)
            *   `$lt`: Less Than (e.g., `age[\$lt]=65`)
            *   `$lte`: Less Than or Equal To (e.g., `stock[\$lte]=5`)
            *   `$like`: SQL LIKE operator (e.g., `name[\$like]=%john%`) (Use URL encoding for `%` -> `%25`)
            *   `$in`: Matches any value in a comma-separated list (e.g., `status[\$in]=pending,processing`)
            *   `$nin`: Not in a comma-separated list (e.g., `category_id[\$nin]=1,2,3`)
        *   Example: `/api/v1/products?price[\$gte]=50&category[\$in]=electronics,books`
    *   **Sorting:** `?sort[fieldName]=ascend|descend`
        *   Example: `/api/v1/orders?sort[order_date]=descend&sort[total_amount]=ascend`
        *   If not provided, defaults to primary key descending.
    *   **Pagination:**
        *   `current` (integer, default: 1): The current page number.
        *   `pageSize` (integer, default: 10, max: 100): Number of records per page.
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

### 6.2 Get Record by ID

*   **Endpoint:** `GET /{TableName}/{id}`
*   **Description:** Retrieves a single record by its primary key.
*   **Example Request:** `GET /api/v1/users/123`
*   **Example Success Response (200 OK):**
    ```json
    {
      "id": 123,
      "username": "john_doe",
      "email": "john.doe@example.com"
    }
    ```
*   **Example Error Response (404 Not Found):**
    ```json
    {
      "error": "Record not found"
    }
    ```

---

### 6.3 Create Record

*   **Endpoint:** `POST /{TableName}`
*   **Description:** Creates a new record in the specified table. (Not applicable to views).
*   **Request Body:** A JSON object representing the record to create.
    ```json
    {
      "name": "New Gadget",
      "category_id": 3,
      "price": 199.99,
      "status": "available"
    }
    ```
*   **Example Success Response (201 Created):**
    The response includes the created record, potentially with database-generated fields like `id` or timestamps.
    ```json
    {
      "id": 101,
      "name": "New Gadget",
      "category_id": 3,
      "price": 199.99,
      "status": "available",
      "created_at": "2023-10-28T10:00:00Z"
    }
    ```
*   **Example Error Response (400 Bad Request - Invalid JSON):**
    ```json
    {
      "error": "Invalid JSON data",
      "details": "some parsing error message"
    }
    ```
*   **Example Error Response (405 Method Not Allowed - if TableName is a View):**
    ```json
    {
      "error": "POST operation only allowed on tables, not VIEW."
    }
    ```

---

### 6.4 Update Record

*   **Endpoint:** `PUT /{TableName}/{id}` or `PATCH /{TableName}/{id}`
*   **Description:** Updates an existing record by its primary key. (Not applicable to views).
    *   `PUT` can be used, but the behavior is typically a partial update (like `PATCH`) for fields provided in the request body.
*   **Request Body:** A JSON object containing the fields to update.
    ```json
    {
      "price": 189.99,
      "status": "in_stock"
    }
    ```
*   **Example Success Response (200 OK):**
    The response includes the full updated record.
    ```json
    {
      "id": 101,
      "name": "New Gadget",
      "category_id": 3,
      "price": 189.99,
      "status": "in_stock",
      "created_at": "2023-10-28T10:00:00Z",
      "updated_at": "2023-10-28T11:30:00Z"
    }
    ```
*   **Example Error Response (404 Not Found):**
    ```json
    {
      "error": "Record not found or no changes made."
    }
    ```

---

### 6.5 Delete Record

*   **Endpoint:** `DELETE /{TableName}/{id}`
*   **Description:** Deletes a record by its primary key. (Not applicable to views).
*   **(Status: Planned - To be implemented next)**
*   **Expected Success Response:** `204 No Content` (with an empty body).
*   **Expected Error Response (404 Not Found):**
    ```json
    {
      "error": "Record not found"
    }
    ```

## 7. Logging

*   Logs are stored in the `./logs/` directory relative to the project root.
*   Log files are named by date (e.g., `2023-10-28.log`).
*   The log level can be configured in `config.json` (`debug`, `info`, `warn`, `error`).
*   Logs include details about incoming requests, executed queries (if GORM logging is enabled at a verbose level), and responses or errors.

## 8. Supported Databases

*   **Currently Implemented & Tested:**
    *   MySQL
    *   PostgreSQL
    *   SQL Server
*   **Planned / In Progress:**
    *   Oracle (Initial GORM dialect integration faced challenges in the development environment; requires further testing and potentially a different driver/dialect combination).

## 9. License

This project is licensed under the MIT License. (Assuming MIT, can be changed).

```
MIT License

Copyright (c) [Year] [Your Name/Organization]

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
