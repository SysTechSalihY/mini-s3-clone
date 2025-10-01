# Mini S3 Clone

A lightweight **Amazon S3 clone** implemented in **Go** using **Fiber**, **GORM**, **MySQL**, and **Redis**.  
Supports file upload/download, bucket versioning, presigned URLs, and task queuing via **Asynq**.

---

## Features

- **Buckets**
  - Create, list, get info, delete
  - Public or private ACLs
  - Optional versioning support
- **Files**
  - Upload, download, delete
  - Versioned files
  - Presigned URLs for secure temporary access
- **Authentication**
  - User signup and email verification
  - Secret key generation for presigned URLs
- **Tasks**
  - Empty bucket
  - Copy bucket
  - Track task progress
- **Middleware**
  - JWT-based authentication
  - Rate limiting via Redis
  - Presigned URL validation

---

## Tech Stack

- **Backend:** Go, Fiber, GORM
- **Database:** MySQL (for metadata)
- **Cache/Queue:** Redis, Asynq
- **File Storage:** Local disk (`./storage`)
- **Email:** AWS SES
- **Other:** UUID, HMAC-SHA256 for presigned URLs

---

## Installation

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- MySQL & Redis (can be run via Docker)
- AWS credentials for SES (optional for email verification)

### Clone the repository

```bash
git clone https://github.com/SysTechSalihY/mini-s3-clone.git
cd mini-s3-clone
