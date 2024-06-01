package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/arcsub/go-moysklad/moysklad"
	"github.com/go-resty/resty/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	Manager struct {
		context    context.Context
		connection *pgxpool.Pool
		client     *moysklad.Client
	}

	Product struct {
		ID            string
		ProductID     int
		MarketplaceID int
		StockQuantity float64
	}
)

// Get the moysklad access token from the environment variable.
var (
	accessToken string
)

func main() {

	// Create a context that is cancellable and cancel it on exit.
	context, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to the database using the given connection string.
	var (
		connection *pgxpool.Pool
		err        error
	)

	if connection, err = pgxpool.New(context, os.Getenv("DATABASE_URL")); err != nil {
		// If the connection could not be established, panic.
		panic(err)
	}
	// Close the connection when the program exits.
	defer connection.Close()

	if accessToken = os.Getenv("MOYSKLAD_TOKEN"); accessToken == "" {
		// If the access token is not set, panic.
		panic("MOYSKLAD_TOKEN is not set")
	}

	// Run the synchronization loop.
	if err := run(context, connection); err != nil {
		// If the synchronization loop returns an error, print it and exit.
		log.Println(err)
	}
}

// Run runs the main synchronization loop. This loop periodically calls the Sync method
// of the manager. If an error occurs during a Sync call, the error is returned
// immediately.
//
// Args:
//
//	context: A cancellable context that can be used to cancel the loop.
//	connection: A connection pool to the database.
//
// Returns:
//
//	If an error occurs during a Sync call, the error is returned immediately.
//	Otherwise, nil is returned.
func run(context context.Context, connection *pgxpool.Pool) error {

	// The polling interval in minutes.
	const (
		polling = 45
	)

	// Create an instance of the manager.
	manager := New(context, connection)

	// The bucket size defines the interval at which products are collected
	// and synchronized. The value is increased by the bucket value after
	// each iteration.
	if err := manager.Sync(1); err != nil {
		// If the initial sync fails, return the error.
		return err
	}

	// Create a ticker that will fire every 15 minutes.
	ticker := time.NewTicker(polling * time.Minute)

	// Stop the ticker when we're done.
	defer ticker.Stop()

	// Run the loop until the context is cancelled.
	for {
		select {
		case <-ticker.C:
			// Call the Sync method periodically.
			if err := manager.Sync(1); err != nil {
				// If an error occurs, return the error.
				return err
			}
		case <-context.Done():
			// If the context is cancelled, the loop should be terminated.
			return nil
		}
	}
}

// New returns an instance of the manager.
//
// Args:
//
//	context context.Context: A cancellable context that can be used to cancel the
//		synchronization loop.
//	connection *pgxpool.Pool: A connection pool to the database.
//
// Returns:
//
//	*Manager: An instance of the manager.
func New(context context.Context, connection *pgxpool.Pool) *Manager {

	// Create a new client using the access token.
	client := moysklad.NewClient().WithTokenAuth(accessToken)

	// Return a new instance of the manager.
	return &Manager{
		context:    context,
		connection: connection,
		client:     client,
	}
}

// Sync synchronizes the local products with the moysklad service.
//
// Sync runs an infinite loop, collecting products from moysklad, updating
// the local database, and then collecting new sale prices, images, and
// attributes for each product. The collected data is then used to update
// the product in moysklad.
//
// Args:
//
//	interval int: The interval at which products are collected and
//		synchronized. The value is increased by the bucket value after each iteration.
//
// Returns:
//
//	error: An error if something goes wrong during the synchronization process.
func (manager *Manager) Sync(bucket int) error {

	var (
		// The interval at which products are collected and synchronized.
		// The value is increased by the bucket value after each iteration.
		interval int
	)

	// The enter service used to interact with the moysklad API.
	documentService := manager.client.Entity().Enter()

	for {
		var (
			products  []Product
			documents *moysklad.List[moysklad.Enter]

			err error
		)

		// The products are collected in batches, with each batch having a size of `bucket`.
		// The `interval` parameter is used to determine the offset at which the next
		// batch of products is collected.
		//
		// Args:
		//
		//	bucket int: The number of products to collect in each batch.
		//	interval int: The offset at which the next batch of products is collected.
		//
		// Returns:
		//
		//	[]Product: A slice of products collected.
		//	error: An error if something goes wrong during the collection process.
		if products, err = manager.CollectProducts(bucket, interval); err != nil {
			return err
		}

		// If no products were collected, break out of the loop.
		if len(products) == 0 {
			break
		}

		// Retrieve the enter documents from moysklad.
		// The documents are returned as a moysklad.List struct.
		//
		// Args:
		//
		//	context.Context: A cancellable context that can be used to cancel the
		//		retrieval of enter documents.
		//	nil: A map of query parameters for filtering the retrieve documents.
		//
		// Returns:
		//
		//	*moysklad.List[moysklad.Enter]: A list of enter documents retrieved from moysklad.
		//	*resty.Response: The HTTP response received from moysklad.
		//	error: An error if something goes wrong during the retrieval process.
		if documents, _, err = documentService.GetList(manager.context, nil); err != nil {
			return err
		}

		if len(documents.Rows) == 0 {
			// Trendyol store
			if _, err = manager.CreateEnter(documentService, "be54bdc2-1448-11ef-0a80-16c50012f572", "640078d9-1eff-11ef-0a80-0c94001ca0fc"); err != nil {
				return err
			}
			// Toyzzshop store
			if _, err = manager.CreateEnter(documentService, "be54bdc2-1448-11ef-0a80-16c50012f572", "6f237006-1eff-11ef-0a80-0665001bf5c6"); err != nil {
				return err
			}
			continue
		}

		// Iterate over the products to be synchronized.
		for _, product := range products {
			productFound := false

			// Determine the store ID based on the product marketplace ID.
			storeID := "640078d9-1eff-11ef-0a80-0c94001ca0fc"
			if product.MarketplaceID == 3 {
				storeID = "6f237006-1eff-11ef-0a80-0665001bf5c6"
			}

			// Iterate over the existing documents to update the product position.
			for _, document := range documents.Rows {
				// Get the positions of the document.
				productPositions, _, err := documentService.GetPositions(manager.context, document.ID, nil)
				if err != nil {
					return err
				}

				// Iterate over the product positions.
				for _, position := range productPositions.Rows {
					// Check if the position corresponds to the product.
					if filepath.Base(*position.Assortment.Meta.Href) == product.ID {
						// Update the product position.
						if _, response, err := documentService.UpdatePosition(manager.context, document.ID, position.ID, manager.Position(product.StockQuantity, product.ID), nil); err != nil {
							Error(response)
							return err
						}
						productFound = true
						break
					}
				}

				if productFound {
					break
				}
			}

			// If the product was not found in any document, create a new document and position.
			if !productFound {
				addedToDocument := false

				// Iterate over the existing documents to add the product position.
				for _, document := range documents.Rows {
					// Check if the document has less than 999 positions.
					if document.Positions.Meta.Size < 999 {
						// Create a new product position.
						if _, response, err := documentService.CreatePosition(manager.context, document.ID, manager.Position(product.StockQuantity, product.ID)); err != nil {
							Error(response)
							return err
						}
						addedToDocument = true
						break
					}
				}

				// If no document had less than 999 positions, create a new document.
				if !addedToDocument {
					document, err := manager.CreateEnter(documentService, "be54bdc2-1448-11ef-0a80-16c50012f572", storeID)
					if err != nil {
						return err
					}

					// Create a new product position in the new document.
					if _, response, err := documentService.CreatePosition(manager.context, document.ID, manager.Position(product.StockQuantity, product.ID)); err != nil {
						Error(response)
						return err
					}
				}
			}
		}

		// Increase the interval at which products are collected and synchronized.
		interval += bucket
	}

	return nil
}

func (manager *Manager) CollectProducts(bucket, interval int) ([]Product, error) {

	// Query the database for the products.
	var (
		rows pgx.Rows

		products []Product
		err      error
	)

	query := `
		SELECT
			mp.id,
			mp.product_id,
			pp.marketplace_id,
			pp.stock_quantity
		FROM
			moysklad.products AS mp
		JOIN
			public.provider_product AS pp ON mp.product_id = pp.id
		WHERE
			pp.stock_quantity > 0
		LIMIT $1
		OFFSET $2;
	`

	if rows, err = manager.connection.Query(manager.context, query, bucket, interval); err != nil {
		return nil, fmt.Errorf("Can't collect products: %w", err)
	}

	// Scan the rows into a slice of Product structs.
	if products, err = pgx.CollectRows(rows, pgx.RowToStructByName[Product]); err != nil {
		return nil, fmt.Errorf("Can't collect products rows: %w", err)
	}

	return products, nil
}

func (manager *Manager) Position(quantity float64, id string) *moysklad.EnterPosition {

	// Create a new moysklad.Store with the given ID.
	return &moysklad.EnterPosition{
		Quantity: moysklad.Float(quantity),
		Assortment: &moysklad.AssortmentPosition{
			Meta: moysklad.Meta{
				// The href is the URL of the product resource.
				Href: moysklad.String("https://api.moysklad.ru/api/remap/1.2/entity/product/" + id),
				// The type is "product".
				Type: moysklad.MetaType("product"),
			},
		},
	}
}

func (manager *Manager) Enter(organization, store string) *moysklad.Enter {

	enter := new(moysklad.Enter)

	enter.Organization = &moysklad.Organization{
		Meta: &moysklad.Meta{
			// The href is the URL of the organization resource.
			Href: moysklad.String("https://api.moysklad.ru/api/remap/1.2/entity/organization/" + organization),
			// The type is "organization".
			Type: moysklad.MetaType("organization"),
		},
	}

	enter.Store = &moysklad.Store{
		Meta: &moysklad.Meta{
			// The href is the URL of the store resource.
			Href: moysklad.String("https://api.moysklad.ru/api/remap/1.2/entity/store/" + store),
			// The type is "store".
			Type: moysklad.MetaType("store"),
		},
	}

	// Create a new moysklad.Enter with the given Name.
	return enter
}

func (manager *Manager) CreateEnter(enterService *moysklad.EnterService, organization, store string) (*moysklad.Enter, error) {

	var (
		enter    *moysklad.Enter
		response *resty.Response

		err error
	)

	if enter, response, err = enterService.Create(manager.context, manager.Enter(organization, store), nil); err != nil {
		Error(response)

		return nil, err
	}

	return enter, nil
}

func Error(response *resty.Response) {

	if err := json.Unmarshal(response.Body(), &response); err != nil {
		log.Println(err)
	}

	log.Println(response)
}
