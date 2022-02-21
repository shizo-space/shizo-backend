package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // import sqlite3 driver

	"fmt"
	"os"
	"time"
)

type MBTileDB struct {
    FileName  string    // name of tile mbtiles file
    DB        *sql.DB   // database connection for mbtiles file
    Timestamp time.Time // timestamp of file, for cache control headers
}

func NewDB(filename string) (*MBTileDB, error) {
    fileStat, err := os.Stat(filename)
    if err != nil {
        return nil, fmt.Errorf("could not read file stats for mbtiles file: %s", filename)
    }

    db, err := sql.Open("sqlite3", filename)
    if err != nil {
        return nil, err
    }

    out := MBTileDB{
        DB:        db,
        FileName:  filename,
        Timestamp: fileStat.ModTime().Round(time.Second), 
    }

    return &out, nil

}

func (tileset *MBTileDB) ReadTile(z uint8, x uint64, y uint64, data *[]byte) error {
    err := tileset.DB.QueryRow("select tile_data from tiles where zoom_level = ? and tile_column = ? and tile_row = ?", z, x, y).Scan(data)
    if err != nil {
        if err == sql.ErrNoRows {
            *data = nil
            return nil
        }
        return err
    }
    return nil
}

func (db *MBTileDB) GetTileData(z uint8, x uint64, y uint64) ([]byte, error) {

    y = (1 << uint64(z)) - 1 - y
    var data []byte
    // flip y to match the spec
    err := db.ReadTile(z, x, y, &data)
    if err != nil {
        return nil, err
    }

    if data == nil || len(data) <= 1 {
        return nil, nil
    }
    return data, nil
}