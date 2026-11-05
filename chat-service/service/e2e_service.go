package service

import (
	"context"
	"fmt"
	"time"

	"social-network-go/chat-service/db"
	"social-network-go/chat-service/model"
	"social-network-go/logger"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type E2ERegisterKeysRequest struct {
	DeviceID        string                `json:"deviceId" binding:"required"`
	DeviceName      string                `json:"deviceName"`
	RegistrationID  uint32                `json:"registrationId" binding:"required"`
	IdentityKeyPub  string                `json:"identityKeyPub" binding:"required"`
	SignedPreKeyID  uint32                `json:"signedPreKeyId" binding:"required"`
	SignedPreKeyPub string                `json:"signedPreKeyPub" binding:"required"`
	SignedPreKeySig string                `json:"signedPreKeySig" binding:"required"`
	OneTimePreKeys  []OneTimePreKeyUpload `json:"oneTimePreKeys" binding:"required"`
}

type OneTimePreKeyUpload struct {
	KeyID     uint32 `json:"keyId" binding:"required"`
	PublicKey string `json:"publicKey" binding:"required"`
}

type E2EAddPreKeysRequest struct {
	DeviceID       string                `json:"deviceId" binding:"required"`
	OneTimePreKeys []OneTimePreKeyUpload `json:"oneTimePreKeys" binding:"required"`
}

type E2EKeyStatusResponse struct {
	IsRegistered        bool               `json:"isRegistered"`
	Devices             []E2EDeviceSummary `json:"devices"`
	HasBackup           bool               `json:"hasBackup"`
	BackupKeyFingerprint string            `json:"backupKeyFingerprint,omitempty"`
}

type E2EDeviceSummary struct {
	DeviceID            string    `json:"deviceId"`
	DeviceName          string    `json:"deviceName"`
	RegistrationID      uint32    `json:"registrationId"`
	RemainingPreKeyCount int64     `json:"remainingPreKeyCount"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type E2EPreKeyBundleResponse struct {
	UserID          string                   `json:"userId"`
	Devices         []E2EDevicePreKeyBundle  `json:"devices"`
}

type E2EDevicePreKeyBundle struct {
	DeviceID        string  `json:"deviceId"`
	RegistrationID  uint32  `json:"registrationId"`
	IdentityKeyPub  string  `json:"identityKeyPub"`
	SignedPreKeyID  uint32  `json:"signedPreKeyId"`
	SignedPreKeyPub string  `json:"signedPreKeyPub"`
	SignedPreKeySig string  `json:"signedPreKeySig"`
	OneTimePreKeyID *uint32 `json:"oneTimePreKeyId,omitempty"`
	OneTimePreKeyPub string `json:"oneTimePreKeyPub,omitempty"`
}

type E2EBackupRequest struct {
	EncryptedVault string `json:"encryptedVault" binding:"required"`
	VaultIV        string `json:"vaultIv" binding:"required"`
	KDFAlgorithm   string `json:"kdfAlgorithm" binding:"required"`
	KDFSalt        string `json:"kdfSalt" binding:"required"`
	KDFMemoryKB    uint32 `json:"kdfMemoryKb" binding:"required"`
	KDFIterations  uint32 `json:"kdfIterations" binding:"required"`
	KDFParallelism uint8  `json:"kdfParallelism" binding:"required"`
	KeyFingerprint string `json:"keyFingerprint" binding:"required"`
	Version        int    `json:"version"`
}

type E2EGroupSenderKeyUpload struct {
	RecipientID        string           `json:"recipientId" binding:"required"`
	RecipientDeviceID  string           `json:"recipientDeviceId" binding:"required"`
	KeyGeneration      uint32           `json:"keyGeneration"`
	EncryptedSenderKey string           `json:"encryptedSenderKey" binding:"required"`
	E2EHeader          *model.E2EHeader `json:"e2eHeader,omitempty"`
}

type E2EGroupSenderKeysBatchRequest struct {
	SenderDeviceID string                    `json:"senderDeviceId" binding:"required"`
	SenderKeys     []E2EGroupSenderKeyUpload `json:"senderKeys" binding:"required"`
}

func (s *ChatService) RegisterDeviceKeys(ctx context.Context, userID string, req *E2ERegisterKeysRequest) error {
	if db.DeviceKeysCollection == nil || db.OneTimePreKeysCollection == nil {
		return fmt.Errorf("database collections not initialized")
	}

	now := time.Now()
	docID := fmt.Sprintf("%s:%s", userID, req.DeviceID)

	bundle := model.DeviceKeyBundle{
		ID:              docID,
		UserID:          userID,
		DeviceID:        req.DeviceID,
		DeviceName:      req.DeviceName,
		RegistrationID:  req.RegistrationID,
		IdentityKeyPub:  req.IdentityKeyPub,
		SignedPreKeyID:  req.SignedPreKeyID,
		SignedPreKeyPub: req.SignedPreKeyPub,
		SignedPreKeySig: req.SignedPreKeySig,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	opts := options.Update().SetUpsert(true)
	_, err := db.DeviceKeysCollection.UpdateOne(ctx, bson.M{"_id": docID}, bson.M{"$set": bundle}, opts)
	if err != nil {
		logger.Err(err).Error("Failed to upsert device key bundle for %s", docID)
		return err
	}

	// Insert One-Time PreKeys
	if len(req.OneTimePreKeys) > 0 {
		var preKeyDocs []interface{}
		for _, opk := range req.OneTimePreKeys {
			preKeyDocs = append(preKeyDocs, model.OneTimePreKey{
				ID:        fmt.Sprintf("%s:%s:%d", userID, req.DeviceID, opk.KeyID),
				UserID:    userID,
				DeviceID:  req.DeviceID,
				KeyID:     opk.KeyID,
				PublicKey: opk.PublicKey,
				CreatedAt: now,
			})
		}
		// Ordered false so duplicates won't stop other keys from being inserted
		_, err = db.OneTimePreKeysCollection.InsertMany(ctx, preKeyDocs, options.InsertMany().SetOrdered(false))
		if err != nil && !mongo.IsDuplicateKeyError(err) {
			logger.Err(err).Warn("Some one-time prekeys could not be inserted for %s: %v", docID, err)
		}
	}

	return nil
}

func (s *ChatService) AddOneTimePreKeys(ctx context.Context, userID string, req *E2EAddPreKeysRequest) (int64, error) {
	if db.OneTimePreKeysCollection == nil {
		return 0, fmt.Errorf("database collection not initialized")
	}

	now := time.Now()
	var preKeyDocs []interface{}
	for _, opk := range req.OneTimePreKeys {
		preKeyDocs = append(preKeyDocs, model.OneTimePreKey{
			ID:        fmt.Sprintf("%s:%s:%d", userID, req.DeviceID, opk.KeyID),
			UserID:    userID,
			DeviceID:  req.DeviceID,
			KeyID:     opk.KeyID,
			PublicKey: opk.PublicKey,
			CreatedAt: now,
		})
	}

	if len(preKeyDocs) > 0 {
		_, _ = db.OneTimePreKeysCollection.InsertMany(ctx, preKeyDocs, options.InsertMany().SetOrdered(false))
	}

	count, _ := db.OneTimePreKeysCollection.CountDocuments(ctx, bson.M{"user_id": userID, "device_id": req.DeviceID})
	return count, nil
}

func (s *ChatService) GetE2EKeyStatus(ctx context.Context, userID string) (*E2EKeyStatusResponse, error) {
	if db.DeviceKeysCollection == nil {
		return &E2EKeyStatusResponse{IsRegistered: false}, nil
	}

	cursor, err := db.DeviceKeysCollection.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var bundles []model.DeviceKeyBundle
	if err := cursor.All(ctx, &bundles); err != nil {
		return nil, err
	}

	if len(bundles) == 0 {
		return &E2EKeyStatusResponse{IsRegistered: false}, nil
	}

	var devices []E2EDeviceSummary
	for _, b := range bundles {
		count := int64(0)
		if db.OneTimePreKeysCollection != nil {
			count, _ = db.OneTimePreKeysCollection.CountDocuments(ctx, bson.M{"user_id": userID, "device_id": b.DeviceID})
		}
		devices = append(devices, E2EDeviceSummary{
			DeviceID:            b.DeviceID,
			DeviceName:          b.DeviceName,
			RegistrationID:      b.RegistrationID,
			RemainingPreKeyCount: count,
			UpdatedAt:           b.UpdatedAt,
		})
	}

	hasBackup := false
	var keyFingerprint string
	if db.KeyBackupsCollection != nil {
		var backup model.KeyBackupRecord
		if err := db.KeyBackupsCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&backup); err == nil {
			hasBackup = true
			keyFingerprint = backup.KeyFingerprint
		}
	}

	return &E2EKeyStatusResponse{
		IsRegistered:        true,
		Devices:             devices,
		HasBackup:           hasBackup,
		BackupKeyFingerprint: keyFingerprint,
	}, nil
}

func (s *ChatService) GetPreKeyBundle(ctx context.Context, targetUserID string) (*E2EPreKeyBundleResponse, error) {
	if db.DeviceKeysCollection == nil {
		return nil, fmt.Errorf("database collections not initialized")
	}

	cursor, err := db.DeviceKeysCollection.Find(ctx, bson.M{"user_id": targetUserID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var bundles []model.DeviceKeyBundle
	if err := cursor.All(ctx, &bundles); err != nil {
		return nil, err
	}

	if len(bundles) == 0 {
		return nil, fmt.Errorf("USER_KEYS_NOT_FOUND")
	}

	var deviceBundles []E2EDevicePreKeyBundle
	for _, b := range bundles {
		devBundle := E2EDevicePreKeyBundle{
			DeviceID:        b.DeviceID,
			RegistrationID:  b.RegistrationID,
			IdentityKeyPub:  b.IdentityKeyPub,
			SignedPreKeyID:  b.SignedPreKeyID,
			SignedPreKeyPub: b.SignedPreKeyPub,
			SignedPreKeySig: b.SignedPreKeySig,
		}

		// Atomically fetch and delete one One-Time PreKey for this device
		if db.OneTimePreKeysCollection != nil {
			var opk model.OneTimePreKey
			findErr := db.OneTimePreKeysCollection.FindOneAndDelete(ctx, bson.M{
				"user_id":   targetUserID,
				"device_id": b.DeviceID,
			}).Decode(&opk)

			if findErr == nil {
				keyId := opk.KeyID
				devBundle.OneTimePreKeyID = &keyId
				devBundle.OneTimePreKeyPub = opk.PublicKey

				// Check remaining count in background to notify target user if low
				go func(uid, did string) {
					checkCtx, checkCancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer checkCancel()
					remaining, err := db.OneTimePreKeysCollection.CountDocuments(checkCtx, bson.M{
						"user_id":   uid,
						"device_id": did,
					})
					if err == nil && remaining < 15 {
						s.SendToUser(uid, map[string]interface{}{
							"command":        "E2E_LOW_PREKEYS",
							"deviceId":       did,
							"remainingCount": remaining,
						})
					}
				}(targetUserID, b.DeviceID)
			}
		}

		deviceBundles = append(deviceBundles, devBundle)
	}

	return &E2EPreKeyBundleResponse{
		UserID:  targetUserID,
		Devices: deviceBundles,
	}, nil
}

func (s *ChatService) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	docID := fmt.Sprintf("%s:%s", userID, deviceID)
	if db.DeviceKeysCollection != nil {
		_, _ = db.DeviceKeysCollection.DeleteOne(ctx, bson.M{"_id": docID})
	}
	if db.OneTimePreKeysCollection != nil {
		_, _ = db.OneTimePreKeysCollection.DeleteMany(ctx, bson.M{"user_id": userID, "device_id": deviceID})
	}
	return nil
}

func (s *ChatService) SaveKeyBackup(ctx context.Context, userID string, req *E2EBackupRequest) error {
	if db.KeyBackupsCollection == nil {
		return fmt.Errorf("database collection not initialized")
	}

	rec := model.KeyBackupRecord{
		UserID:         userID,
		EncryptedVault: req.EncryptedVault,
		VaultIV:        req.VaultIV,
		KDFAlgorithm:   req.KDFAlgorithm,
		KDFSalt:        req.KDFSalt,
		KDFMemoryKB:    req.KDFMemoryKB,
		KDFIterations:  req.KDFIterations,
		KDFParallelism: req.KDFParallelism,
		KeyFingerprint: req.KeyFingerprint,
		Version:        req.Version,
		UpdatedAt:      time.Now(),
	}

	opts := options.Update().SetUpsert(true)
	_, err := db.KeyBackupsCollection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": rec}, opts)
	return err
}

func (s *ChatService) GetKeyBackup(ctx context.Context, userID string) (*model.KeyBackupRecord, error) {
	if db.KeyBackupsCollection == nil {
		return nil, fmt.Errorf("database collection not initialized")
	}

	var rec model.KeyBackupRecord
	err := db.KeyBackupsCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&rec)
	if err != nil {
		return nil, fmt.Errorf("BACKUP_NOT_FOUND")
	}
	return &rec, nil
}

func (s *ChatService) SaveGroupSenderKeys(ctx context.Context, senderID string, chatID string, req *E2EGroupSenderKeysBatchRequest) error {
	if db.GroupSenderKeysCollection == nil {
		return fmt.Errorf("database collection not initialized")
	}

	now := time.Now()
	logger.Info("🔑 [E2E-BE] SaveGroupSenderKeys chatID=%s senderID=%s senderDeviceID=%s count=%d",
		chatID, senderID, req.SenderDeviceID, len(req.SenderKeys))

	var docs []interface{}
	for _, sk := range req.SenderKeys {
		id := fmt.Sprintf("%s:%s:%s:%s:%d", chatID, senderID, req.SenderDeviceID, sk.RecipientID, sk.KeyGeneration)
		logger.Info("🔑 [E2E-BE]   -> Storing key record: id=%s recipientId=%s recipientDeviceId=%s gen=%d",
			id, sk.RecipientID, sk.RecipientDeviceID, sk.KeyGeneration)
		docs = append(docs, model.GroupSenderKeyRecord{
			ID:                 id,
			ChatID:             chatID,
			SenderID:           senderID,
			SenderDeviceID:     req.SenderDeviceID,
			RecipientID:        sk.RecipientID,
			RecipientDeviceID:  sk.RecipientDeviceID,
			KeyGeneration:      sk.KeyGeneration,
			EncryptedSenderKey: sk.EncryptedSenderKey,
			E2EHeader:          sk.E2EHeader,
			CreatedAt:          now,
		})
	}

	if len(docs) > 0 {
		opts := options.Update().SetUpsert(true)
		for _, d := range docs {
			rec := d.(model.GroupSenderKeyRecord)
			_, err := db.GroupSenderKeysCollection.UpdateOne(ctx, bson.M{"_id": rec.ID}, bson.M{"$set": rec}, opts)
			if err != nil {
				logger.Err(err).Error("🔑 [E2E-BE] Failed to upsert group sender key: %s", rec.ID)
			}
		}
	}

	return nil
}

func (s *ChatService) GetGroupSenderKeys(ctx context.Context, recipientID, recipientDeviceID, chatID string) ([]model.GroupSenderKeyRecord, error) {
	if db.GroupSenderKeysCollection == nil {
		return []model.GroupSenderKeyRecord{}, nil
	}

	logger.Info("🔑 [E2E-BE] GetGroupSenderKeys request: chatID=%s recipientID=%s recipientDeviceID=%s",
		chatID, recipientID, recipientDeviceID)

	filter := bson.M{
		"chat_id":      chatID,
		"recipient_id": recipientID,
	}
	if recipientDeviceID != "" {
		filter["recipient_device_id"] = recipientDeviceID
	}

	opts := options.Find().SetSort(bson.M{"key_generation": -1})
	cursor, err := db.GroupSenderKeysCollection.Find(ctx, filter, opts)
	if err != nil {
		logger.Err(err).Error("🔑 [E2E-BE] Find error in GetGroupSenderKeys")
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []model.GroupSenderKeyRecord
	if err := cursor.All(ctx, &list); err != nil {
		logger.Err(err).Error("🔑 [E2E-BE] Cursor.All error in GetGroupSenderKeys")
		return nil, err
	}
	logger.Info("🔑 [E2E-BE] GetGroupSenderKeys found %d records for recipientID=%s", len(list), recipientID)
	for _, r := range list {
		logger.Info("🔑 [E2E-BE]   -> Found key from senderID=%s (senderDevice=%s, gen=%d)",
			r.SenderID, r.SenderDeviceID, r.KeyGeneration)
	}
	return list, nil
}
