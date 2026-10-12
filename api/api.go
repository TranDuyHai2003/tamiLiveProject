// Package api chứa hợp đồng API (OpenAPI), nhúng vào binary để server tự phục vụ tại /openapi.yaml.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte

// DocsHTML: trang Swagger UI đọc /openapi.yaml. Chỉ bật ngoài production.
const DocsHTML = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>TamiLive API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: "/openapi.yaml", dom_id: "#swagger-ui", persistAuthorization: true });
  </script>
</body>
</html>`
