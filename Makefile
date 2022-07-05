
generate_rpc_client:
	swagger generate client -t services/rpc_client -f ./services/rpc_client/client.yml -A tezosrpc

dev-init:
	docker compose -f docker-compose.dev.yml up -d api-db
	docker compose -f docker-compose.dev.yml up -d api-pgadmin
	docker compose -f docker-compose.dev.yml up	-d ghost-db
	@sleep 5
	docker-compose -f docker-compose.dev.yml exec -T ghost-db psql -U tzkt postgres -c '\l'
	docker-compose -f docker-compose.dev.yml exec -T ghost-db dropdb -U tzkt --if-exists tzkt_db
	docker-compose -f docker-compose.dev.yml exec -T ghost-db createdb -U tzkt -T template0 tzkt_db
	docker-compose -f docker-compose.dev.yml exec -T ghost-db apt update
	docker-compose -f docker-compose.dev.yml exec -T ghost-db apt install -y wget
	docker-compose -f docker-compose.dev.yml exec -T ghost-db wget "https://tzkt.fra1.digitaloceanspaces.com/snapshots/tzkt_v1.9_ghostnet.backup" -O tzkt_db.backup
	docker-compose -f docker-compose.dev.yml exec -T ghost-db pg_restore -U tzkt -O -x -v -d tzkt_db -e -j 4 tzkt_db.backup
	docker-compose -f docker-compose.dev.yml exec -T ghost-db rm tzkt_db.backup
	docker-compose -f docker-compose.dev.yml exec -T ghost-db apt autoremove --purge -y wget
	docker compose -f docker-compose.dev.yml pull

dev-db-start:
	docker compose -f docker-compose.dev.yml up -d api-db
	docker compose -f docker-compose.dev.yml up -d api-pgadmin
	docker compose -f docker-compose.dev.yml up -d ghost-db

dev-start:
	docker compose -f docker-compose.dev.yml up -d

dev-stop:
	docker compose -f docker-compose.dev.yml down

dev-clean:
	docker compose -f docker-compose.dev.yml down --volumes
