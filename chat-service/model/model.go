package model

import "time"

type E2EHeader struct {
	Version           int     `json:"version" bson:"version"`
	Algorithm         string  `json:"algorithm" bson:"algorithm"` // X3DH_DR_V1, SENDER_KEY_V1
	SenderDeviceID    string  `json:"senderDeviceId" bson:"sender_device_id"`
	RecipientDeviceID string  `json:"recipientDeviceId,omitempty" bson:"recipient_device_id,omitempty"`
	SenderIdentityKey string  `json:"senderIdentityKey,omitempty" bson:"sender_identity_key,omitempty"`
	EphemeralKey      string  `json:"ephemeralKey,omitempty" bson:"ephemeral_key,omitempty"`
	SignedPreKeyID    *uint32 `json:"signedPreKeyId,omitempty" bson:"signed_pre_key_id,omitempty"`
	OneTimePreKeyID   *uint32 `json:"oneTimePreKeyId,omitempty" bson:"one_time_pre_key_id,omitempty"`
	ChainIndex        uint32  `json:"chainIndex" bson:"chain_index"`
	PreviousChainLen  uint32  `json:"previousChainLen,omitempty" bson:"previous_chain_len,omitempty"`
	KeyGeneration     uint32  `json:"keyGeneration,omitempty" bson:"key_generation,omitempty"`
	IV                string  `json:"iv" bson:"iv"`
	Signature         string  `json:"signature,omitempty" bson:"signature,omitempty"`
}

type DeviceKeyBundle struct {
	ID              string    `json:"id" bson:"_id"` // "<userId>:<deviceId>"
	UserID          string    `json:"userId" bson:"user_id"`
	DeviceID        string    `json:"deviceId" bson:"device_id"`
	DeviceName      string    `json:"deviceName" bson:"device_name"`
	RegistrationID  uint32    `json:"registrationId" bson:"registration_id"`
	IdentityKeyPub  string    `json:"identityKeyPub" bson:"identity_key_pub"`
	SignedPreKeyID  uint32    `json:"signedPreKeyId" bson:"signed_pre_key_id"`
	SignedPreKeyPub string    `json:"signedPreKeyPub" bson:"signed_pre_key_pub"`
	SignedPreKeySig string    `json:"signedPreKeySig" bson:"signed_pre_key_sig"`
	CreatedAt       time.Time `json:"createdAt" bson:"created_at"`
	UpdatedAt       time.Time `json:"updatedAt" bson:"updated_at"`
}

type OneTimePreKey struct {
	ID        string    `json:"id" bson:"_id"` // "<userId>:<deviceId>:<keyId>"
	UserID    string    `json:"userId" bson:"user_id"`
	DeviceID  string    `json:"deviceId" bson:"device_id"`
	KeyID     uint32    `json:"keyId" bson:"key_id"`
	PublicKey string    `json:"publicKey" bson:"public_key"`
	CreatedAt time.Time `json:"createdAt" bson:"created_at"`
}

type GroupSenderKeyRecord struct {
	ID                 string     `json:"id" bson:"_id"`
	ChatID             string     `json:"chatId" bson:"chat_id"`
	SenderID           string     `json:"senderId" bson:"sender_id"`
	SenderDeviceID     string     `json:"senderDeviceId" bson:"sender_device_id"`
	RecipientID        string     `json:"recipientId" bson:"recipient_id"`
	RecipientDeviceID  string     `json:"recipientDeviceId" bson:"recipient_device_id"`
	KeyGeneration      uint32     `json:"keyGeneration" bson:"key_generation"`
	EncryptedSenderKey string     `json:"encryptedSenderKey" bson:"encrypted_sender_key"`
	E2EHeader          *E2EHeader `json:"e2eHeader,omitempty" bson:"e2e_header,omitempty"`
	CreatedAt          time.Time  `json:"createdAt" bson:"created_at"`
}

type KeyBackupRecord struct {
	UserID         string    `json:"userId" bson:"_id"`
	EncryptedVault string    `json:"encryptedVault" bson:"encrypted_vault"`
	VaultIV        string    `json:"vaultIv" bson:"vault_iv"`
	KDFAlgorithm   string    `json:"kdfAlgorithm" bson:"kdf_algorithm"`
	KDFSalt        string    `json:"kdfSalt" bson:"kdf_salt"`
	KDFMemoryKB    uint32    `json:"kdfMemoryKb" bson:"kdf_memory_kb"`
	KDFIterations  uint32    `json:"kdfIterations" bson:"kdf_iterations"`
	KDFParallelism uint8     `json:"kdfParallelism" bson:"kdf_parallelism"`
	KeyFingerprint string    `json:"keyFingerprint" bson:"key_fingerprint"`
	Version        int       `json:"version" bson:"version"`
	UpdatedAt      time.Time `json:"updatedAt" bson:"updated_at"`
}

type Message struct {
	ID             string     `json:"id" bson:"_id"`
	ChatID         string     `json:"chatId" bson:"chat_id"`
	SenderID       string     `json:"senderId" bson:"sender_id"`
	RecipientID    string     `json:"recipientId" bson:"recipient_id"`
	Content        string     `json:"content" bson:"content"`
	Attachment     string     `json:"attachment,omitempty" bson:"attachment,omitempty"`
	AttachmentName string     `json:"attachmentName,omitempty" bson:"attachment_name,omitempty"`
	Timestamp      time.Time  `json:"timestamp" bson:"timestamp"`
	Type           string     `json:"type" bson:"type"`     // TEXT, FILE, GIF, VOICE, CALL
	Status         string     `json:"status" bson:"status"` // SENT, READ
	ReadBy         []string   `json:"readBy,omitempty" bson:"read_by,omitempty"`
	IsEncrypted    bool       `json:"isEncrypted" bson:"is_encrypted"`
	IsDeleted      bool       `json:"isDeleted" bson:"is_deleted"`
	E2EHeader      *E2EHeader `json:"e2eHeader,omitempty" bson:"e2e_header,omitempty"`
	CallID         string     `json:"callId,omitempty" bson:"call_id,omitempty"`
	CallAt         *time.Time `json:"callAt,omitempty" bson:"call_at,omitempty"`
	AnswerAt       *time.Time `json:"answerAt,omitempty" bson:"answer_at,omitempty"`
	EndAt          *time.Time `json:"endAt,omitempty" bson:"end_at,omitempty"`
	IsAnswered     *bool      `json:"isAnswered,omitempty" bson:"is_answered,omitempty"`
	IsVideoCall    *bool      `json:"isVideoCall,omitempty" bson:"is_video_call,omitempty"`
	IsRejected     *bool      `json:"isRejected,omitempty" bson:"is_rejected,omitempty"`
}
