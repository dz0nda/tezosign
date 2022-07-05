
generate_rpc_client:
	swagger generate client -t services/rpc_client -f ./services/rpc_client/client.yml -A tezosrpc

dev-init:
	docker compose -f docker-compose.dev.yml up -d api-db
	docker compose -f docker-compose.dev.yml up -d api-pgadmin
	docker compose -f docker-compose.dev.yml up   -d ithaca-db
	@sleep 5
	docker compose -f docker-compose.dev.yml exec -T ithaca-db psql -U tzkt -d tzkt_db -c '\l'
	docker compose -f docker-compose.dev.yml exec -T ithaca-db dropdb -U tzkt --if-exists tzkt_db
	docker compose -f docker-compose.dev.yml exec -T ithaca-db createdb -U tzkt -T template0 tzkt_db
	docker compose -f docker-compose.dev.yml exec -T ithaca-db apt update
	docker compose -f docker-compose.dev.yml exec -T ithaca-db apt install -y wget
	docker compose -f docker-compose.dev.yml exec -T ithaca-db wget "https://tzkt.fra1.digitaloceanspaces.com/snapshots/tzkt_v1.8_ithacanet.backup" -O tzkt_db.backup
	docker compose -f docker-compose.dev.yml exec -T ithaca-db pg_restore -U tzkt -O -x -v -d tzkt_db -e -j 4 tzkt_db.backup
	docker compose -f docker-compose.dev.yml exec -T ithaca-db rm tzkt_db.backup
	docker compose -f docker-compose.dev.yml exec -T ithaca-db apt autoremove --purge -y wget
	docker compose -f docker-compose.dev.yml pull

dev-db-start:
	docker compose -f docker-compose.dev.yml up -d api-db
	docker compose -f docker-compose.dev.yml up -d api-pgadmin
	docker compose -f docker-compose.dev.yml up   -d ithaca-db

dev-start:
	docker compose -f docker-compose.dev.yml up -d

dev-stop:
	docker compose -f docker-compose.dev.yml down

dev-clean:
	docker compose -f docker-compose.dev.yml down --volumes
