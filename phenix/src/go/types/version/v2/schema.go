package v2

var OpenAPI = []byte(`
openapi: "3.0.0"
info:
  title: phenix
  version: "2.0"
paths: {}
components:
  schemas:
    Scenario:
      type: object
      required:
      - apps
      properties:
        apps:
          type: array
          items:
            type: object
            required:
            - name
            properties:
              name:
                type: string
                minLength: 1
              assetDir:
                type: string
              metadata:
                type: object
                additionalProperties: true
              hosts:
                type: array
                items:
                  type: object
                  required:
                  - hostname
                  - metadata
                  properties:
                    hostname:
                      type: string
                      minLength: 1
                    metadata:
                      type: object
                      additionalProperties: true
`)
