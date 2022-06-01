TZSIGN_DEV = docker-compose.tzsign.dev.yml
TZKT_DEV = docker-compose.tzkt.dev.yml

generate_rpc_client:
	swagger generate client -t services/rpc_client -f ./services/rpc_client/client.yml -A tezosrpc

tzsign-dev-init:
	docker compose -f ${TZSIGN_DEV} up -d api-db
	docker compose -f ${TZSIGN_DEV} up -d api-pgadmin

tzsign-dev-start:
	docker compose -f ${TZSIGN_DEV} up -d

tzsign-dev-stop:
	docker compose -f ${TZSIGN_DEV} down --volumes

tzkt-dev-init:
	docker compose -f ${TZKT_DEV} up   -d ithaca-db
	@sleep 2
	docker compose -f ${TZKT_DEV} exec -T ithaca-db psql -U tzkt -d tzkt_db -c '\l'
	docker compose -f ${TZKT_DEV} exec -T ithaca-db dropdb -U tzkt --if-exists tzkt_db
	docker compose -f ${TZKT_DEV} exec -T ithaca-db createdb -U tzkt -T template0 tzkt_db
	docker compose -f ${TZKT_DEV} exec -T ithaca-db apt update
	docker compose -f ${TZKT_DEV} exec -T ithaca-db apt install -y wget
	docker compose -f ${TZKT_DEV} exec -T ithaca-db wget "https://tzkt.fra1.digitaloceanspaces.com/snapshots/tzkt_v1.8_ithacanet.backup" -O tzkt_db.backup
	docker compose -f ${TZKT_DEV} exec -T ithaca-db pg_restore -U tzkt -O -x -v -d tzkt_db -e -j 4 tzkt_db.backup
	docker compose -f ${TZKT_DEV} exec -T ithaca-db rm tzkt_db.backup
	docker compose -f ${TZKT_DEV} exec -T ithaca-db apt autoremove --purge -y wget
	docker compose pull

tzkt-dev-start:
	docker compose -f ${TZKT_DEV} up -d

tzkt-dev-stop:
	docker compose -f ${TZKT_DEV} down --volumes

tzkt-dev-db-start:
	docker compose -f ${TZKT_DEV} up -d ithaca-db
