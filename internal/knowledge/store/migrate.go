package store

import bolt "go.etcd.io/bbolt"

// migrate upgrades a file from version to SchemaVersion. Version 0 is a
// new file. Each future version adds one step here, in order.
func migrate(tx *bolt.Tx, version uint64) error {
	if version == 0 {
		for _, collection := range collections {
			if _, err := tx.CreateBucketIfNotExists(
				[]byte(collection),
			); err != nil {
				return err
			}
		}
	}
	return nil
}
