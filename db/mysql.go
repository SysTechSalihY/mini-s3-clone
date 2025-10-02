package db

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type User struct {
	ID           string `gorm:"primaryKey;type:varchar(36)"`
	Email        string `gorm:"unique;type:varchar(254);not null"`
	SecretKey    string `gorm:"type:varchar(64);not null"`
	AccessKey    string `gorm:"unique;type:varchar(32);not null"`
	PasswordHash string `gorm:"type:varchar(255);not null"`
	IsVerified   bool   `gorm:"default:false"`
	UserRole     string `gorm:"type:varchar(16);default:'user';not null"`
	// UserRole     string    `gorm:"type:enum('user','admin');default:'user';not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	Buckets   []Bucket  `gorm:"foreignKey:UserID"`
}

type Bucket struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)"`
	BucketName string     `gorm:"unique;type:varchar(64);not null"`
	UserID     string     `gorm:"type:varchar(36);not null;index"`
	Region     string     // `gorm:"type:enum('USA','TR','CHINA','JP');not null"`
	ACL        *string    //`gorm:"type:enum('private','public-read');default:'private';not null"`
	Versioning bool       `gorm:"default:false"`
	Quota      *int64     `gorm:"default:null"` // bytes, optional
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  *time.Time `gorm:"autoUpdateTime"`

	Files []File `gorm:"foreignKey:BucketID;constraint:OnDelete:CASCADE"`
}

type File struct {
	ID          string     `gorm:"primaryKey;type:varchar(36)"`
	BucketID    string     `gorm:"type:varchar(36);not null;index:idx_bucket_file"`
	FileName    string     `gorm:"type:varchar(255);not null;index:idx_bucket_file"`
	Size        int64      `gorm:"not null"`
	ContentType string     `gorm:"type:varchar(128)"`
	VersionID   string     `gorm:"type:varchar(36);default:null;index"` // for versioning
	IsLatest    bool       `gorm:"default:true"`                        // marks latest version
	CreatedAt   time.Time  `gorm:"autoCreateTime;index:idx_bucket_created"`
	UpdatedAt   *time.Time `gorm:"autoUpdateTime"`

	Bucket Bucket `gorm:"foreignKey:BucketID;constraint:OnDelete:CASCADE"`
}

type EmailVerification struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)"`
	UserID    string    `gorm:"type:varchar(36);not null"`
	Token     string    `gorm:"unique;type:varchar(64);not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

type Task struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)"`
	UserID     string     `gorm:"type:varchar(36);not null"`
	Type       string     // `gorm:"type:enum('copy','empty');not null"`
	BucketSrc  *string    `gorm:"type:varchar(64)"`
	BucketDest *string    `gorm:"type:varchar(64)"`
	Status     string     //`gorm:"type:enum('running','completed','failed');default:'running'"`
	Progress   int        `gorm:"default:0"`
	Message    string     `gorm:"type:varchar(255)"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  *time.Time `gorm:"autoUpdateTime"`
}

var DB *gorm.DB

func ConnectDb() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("DATABASE_URL environment variable is not set")
		return fmt.Errorf("DATABASE_URL not set")
	}

	var err error

	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.WithError(err).Error("Failed to connect to MySQL database")
		return err
	}

	// Migrate all tables
	err = DB.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(
		&User{},
		&Bucket{},
		&File{},
		&EmailVerification{},
		&Task{},
	)
	if err != nil {
		log.WithError(err).Error("Failed to auto-migrate tables")
		return err
	}

	log.Info("Database connected successfully and tables migrated")
	return nil
}
