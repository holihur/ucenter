// Command purge-tenants removes tenants whose retention window has expired.
package main

import (
	"context"
	"log"
	"time"

	"ucenter/internal/config"
	"ucenter/internal/store"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	st, err := store.New(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close(context.Background())
	tenants, err := st.ListPurgeableTenants(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for i := range tenants {
		if err := st.PurgeTenant(ctx, tenants[i].ID); err != nil {
			log.Printf("purge %s failed: %v", tenants[i].Slug, err)
			continue
		}
		log.Printf("purged tenant %s (%s)", tenants[i].Slug, tenants[i].ID.Hex())
	}
	log.Printf("purged %d tenant(s)", len(tenants))
}
