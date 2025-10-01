-- USERS (already defined, keep as is)
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    secret_key VARCHAR(64) NOT NULL,
    access_key VARCHAR(32) NOT NULL UNIQUE,
    is_verified BOOLEAN DEFAULT FALSE,
    user_role ENUM('user', 'admin') NOT NULL DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- EMAIL VERIFICATIONS
CREATE TABLE IF NOT EXISTS email_verifications (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    token VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- BUCKETS with ACL, versioning, quota
CREATE TABLE IF NOT EXISTS buckets (
    id VARCHAR(36) PRIMARY KEY,
    bucket_name VARCHAR(64) NOT NULL UNIQUE,
    user_id VARCHAR(36) NOT NULL,
    region ENUM('USA', 'TR', 'CHINA', 'JP') NOT NULL,
    acl ENUM('private', 'public-read') NOT NULL DEFAULT 'private',
    versioning BOOLEAN DEFAULT FALSE,
    quota BIGINT DEFAULT NULL,
    -- bytes, optional
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- FILES (objects) with versioning support
CREATE TABLE IF NOT EXISTS files (
    id VARCHAR(36) PRIMARY KEY,
    bucket_id VARCHAR(36) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    size BIGINT NOT NULL,
    content_type VARCHAR(128) DEFAULT NULL,
    version_id VARCHAR(36) DEFAULT NULL,
    -- version identifier if versioning is enabled
    is_latest BOOLEAN DEFAULT TRUE,
    -- true for the latest version of the file
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NULL,
    FOREIGN KEY (bucket_id) REFERENCES buckets(id) ON DELETE CASCADE
);

-- FILES INDEXES
CREATE INDEX idx_bucket_file ON files(bucket_id, file_name);

CREATE INDEX idx_bucket_created ON files(bucket_id, created_at DESC);

CREATE INDEX idx_file_version ON files(bucket_id, file_name, version_id);

-- TAGS (flexible key/value metadata for buckets or files)
CREATE TABLE IF NOT EXISTS tags (
    id VARCHAR(36) PRIMARY KEY,
    resource_type ENUM('bucket', 'file') NOT NULL,
    resource_id VARCHAR(36) NOT NULL,
    -- references bucket.id or file.id
    tag_key VARCHAR(128) NOT NULL,
    tag_value VARCHAR(256) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NULL,
);

CREATE INDEX idx_tags_resource ON tags(resource_type, resource_id);

CREATE UNIQUE INDEX idx_tags_unique ON tags(resource_type, resource_id, tag_key);

-- Tasks 
CREATE TABLE IF NOT EXISTS tasks(
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    type ENUM("copy", "empty") NOT NULL,
    status ENUM("running", "completed", "failed") DEFAULT "running",
    progress INTEGER DEFAULT 0,
    bucket_src VARCHAR(64),
    bucket_dest VARCHAR(64),
    message VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NULL
);