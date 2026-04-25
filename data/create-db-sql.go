package data

var create_db_sql = `

CREATE TABLE "well" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"field" INTEGER,
    "code" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "x" REAL,
    "y" REAL,
	"bushing" REAL
);

CREATE TABLE "field" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "code" TEXT,
    "name" TEXT,
    "parent" INTEGER
);

CREATE TABLE "well_group" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"field" INTEGER,
    "code" TEXT,
    "name" TEXT
	);

CREATE TABLE "well_group_data" (
    "group" INTEGER,
    "well" INTEGER
	);

CREATE INDEX  "well_group_data_index1" ON well_group_data ("group");

`
