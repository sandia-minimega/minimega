package v0

var OpenAPI = []byte(`
openapi: "3.0.0"
info:
  title: phenix
  version: "1.0"
paths: {}
components:
  schemas:
    Topology:
      type: object
      title: Demo Topology
      required:
      - nodes
      properties:
        nodes:
          type: array
          title: Nodes
          items:
            $ref: "#/components/schemas/Node"
    Node:
      type: object
      title: Node
      required:
      - type
      properties:
        type:
          type: string
          title: Node Type
          enum:
          - Firewall
          - Printer
          - Router
          - Server
          - Switch
          - VirtualMachine
          default: VirtualMachine
          example: VirtualMachine
`)
