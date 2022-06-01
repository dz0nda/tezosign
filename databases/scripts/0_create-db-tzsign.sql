-- DROP DATABASE IF EXISTS tzsign_db;

-- CREATE USER tzsign PASSWORD 'qwerty';

-- CREATE DATABASE tzsign_db;

-- GRANT ALL PRIVILEGES ON DATABASE tzsign_db TO tzsign;

\c tzsign_db tzsign;

DROP SCHEMA IF EXISTS msig;
CREATE SCHEMA msig;
SET search_path TO msig;

DROP TABLE IF EXISTS contracts;


-- Init --

create table contracts
(
	ctr_id serial not null
		constraint contracts_pk
		primary key,
	ctr_address varchar(36) not null,
	ctr_last_block_level int
);

create unique index contracts_address_uindex
	on contracts (ctr_address);

create table requests
(
	req_id serial not null
		constraint requests_pk
		primary key,
	ctr_id int not null
		constraint requests_contracts_id_fk
		references contracts,
    req_hash varchar(32) not null,
	req_status varchar default 'wait' not null,
	req_counter int, -- internal msig contract counter
	req_created_at timestamp without time zone default now() not null,
    req_info text not null,
    req_storage_diff text,
    req_nonce int, --field for multi operation txs
    req_network_id varchar not null,
 	req_operation_id varchar(51)
);

create unique index requests_req_operation_id_req_nonce_uindex
	on requests (req_operation_id, req_nonce);

create unique index requests_ctr_id_req_counter_uindex
	on requests (ctr_id, req_counter);

create table signatures
(
	sig_id serial not null
		constraint signatures_pk
		primary key,
	req_id int not null
		constraint signatures_requests_id_fk
		references requests,
	sig_index int not null,
	sig_data varchar not null,
	sig_type varchar not null
);

create unique index signatures_sign_uindex
	on signatures (sig_data);


CREATE VIEW request_json_signatures AS
select req_id,
       json_agg(json_build_object('index', sig_index, 'type', sig_type, 'signature', sig_data)) as signatures
from (select * from signatures order by sig_index, sig_type) AS s
GROUP BY req_id;

CREATE OR REPLACE VIEW request_json_signatures_typed AS
select req_id, sig_type,
       json_agg(json_build_object('index', sig_index, 'type', sig_type, 'signature', sig_data)) as signatures
from (select * from signatures order by sig_index, sig_type) AS s
GROUP BY req_id, sig_type;


-- Auth --

DROP TABLE IF EXISTS auth_tokens;

create table auth_tokens
(
	atn_id serial not null
		constraint auth_requests_pk
		primary key,
	atn_pubkey varchar(55) not null,
    atn_data varchar(64) not null,
    atn_type varchar not null,
    atn_is_used boolean not null,
    atn_expires_at timestamp without time zone not null
);


-- Assets --

DROP TABLE IF EXISTS assets;

create table assets
(
	ast_id serial not null
		constraint asset_pk
		primary key,
    ast_name varchar not null,
    ast_contract_type varchar not null,
    ast_address varchar(36) not null,
    ast_dexter_address varchar(36),
    ast_scale int not null,
    ast_ticker varchar not null,
    ast_token_id int,
    ast_is_active bool default TRUE not null,
    ast_last_block_level int,
    ast_updated_at timestamp default now() not null,
	ctr_id int
		constraint assets_contracts_ctr_id_fk
		references contracts
);

create unique index assets_ctr_id_ast_address_ast_token_id_uindex
	on assets (ctr_id, ast_address,ast_token_id);


-- Vestings --

DROP TABLE IF EXISTS vestings;

create table vestings
(
	vst_id serial not null
		constraint vesting_pk
		primary key,
    vst_name varchar not null,
    vst_address varchar(36) not null,
	ctr_id int
		constraint vestings_contracts_ctr_id_fk
		references contracts
);

create unique index vestings_ctr_id_vst_address_uindex
	on vestings (ctr_id, vst_address);
