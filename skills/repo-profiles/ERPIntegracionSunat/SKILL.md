---
name: ERPIntegracionSunat-profile
description: "Contrato de ejecución para agentes que modifican ERPIntegracionSunat. Trigger: cargar al operar en ERPIntegracionSunat."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
  repo_type: backend
  primary_agent: backend-implementer
  tech_stack: ["Node.js", "Express", "Puppeteer", "dotenv"]
  gitlab_path: SmartClic/erpintegracionsunat
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPIntegracionSunat**.
- Este repo **NO es C#**. Es un servicio Node.js de web scraping.
- Si eres implementador, respeta las reglas de este repo — NO apliques patrones C# aquí.

> **⚠️ Atención:** Este repo es un **servicio de scraping con Puppeteer** para SUNAT. No es un microservicio REST típico. No tiene capas de dominio, no usa CQRS, y no tiene base de datos propia.

## 2. Language Domain Contract

- **Código:** JavaScript (Node.js). Sin TypeScript.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Comentarios:** Solo para lógica de scraping compleja o selectores CSS frágiles.

## 3. Architectural Invariants

Servicio minimalista de **Express + Puppeteer Cluster** para scraping SUNAT.

1. **Sin capas de dominio:** Este repo NO tiene Clean Architecture. Es un servicio simple con rutas Express que ejecutan scraping.
2. **Configuración por entorno:** Usa `dotenv` con archivos `.env.dev`, `.env.crt`, `.env.prd`. Las variables son `NODE_ENV`, `HOST`, `PORT`.
3. **Puppeteer Cluster:** Usa `puppeteer-cluster` para scraping concurrente. No reemplazar por Puppeteer directo sin evaluar carga.
4. **Sin base de datos:** Este servicio no tiene BD propia. Consulta SUNAT y devuelve resultados.
5. **Dockerfile + supervisord:** Desplegado como container con supervisord para manejo de procesos.

## 4. Directory Structure Contract

```
erpintegracionsunat/
├── routes/                → Rutas Express
│   ├── index.js           → Router principal
│   ├── anadirPSE.js       → Ruta de PSE
│   ├── asignaciones.router.js    → Ruta de asignaciones
│   └── conectividades.router.js  → Ruta de conectividades
├── devops/                → Deployment config
├── main.js                → Entry point Express
├── package.json           → Dependencies (Puppeteer, Express, dotenv)
├── Dockerfile             → Container config
├── supervisord.conf       → Process manager
├── .env.dev / .env.crt / .env.prd → Config por entorno
└── Jenkinsfile-crt.groovy → CI/CD
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Rutas nuevas** | Archivo separado en `routes/` + registrado en `routes/index.js` | Todo inline en `main.js` |
| **Scraping** | Usar `puppeteer-cluster` para concurrencia | `puppeteer.launch()` directo por cada request |
| **Configuración** | `process.env.PORT` via dotenv | Hardcodear puerto o URLs |
| **Selectores CSS** | Comentar selectores frágiles de SUNAT | Selectores sin documentar que se rompen sin aviso |

## 6. Testing Contract

- **No hay tests** en este repo.
- **Build:** `npm install` + `npm run start:prod`.
- Los cambios de scraping son frágiles — verificar manualmente contra SUNAT antes de mergear.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que las rutas nuevas están registradas en `routes/index.js`.
- Confirmación de que `npm install` y arranque exitoso.
- Si se modificaron selectores CSS: documentar cuáles y por qué (SUNAT puede cambiar su HTML).
