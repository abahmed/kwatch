package store

import bolt "go.etcd.io/bbolt"

// migrate upgrades a file from version to SchemaVersion. Version 0 is a
// new file. Each future version adds one step here, in order.
func migrate(tx *bolt.Tx, _ uint64) error {
	// Collections are created on every open, so a collection added before
	// the first stable release needs no schema bump.
	for _, collection := range collections {
		if _, err := tx.CreateBucketIfNotExists(
			[]byte(collection),
		); err != nil {
			return err
		}
	}
	return nil
}
