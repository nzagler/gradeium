package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/nzagler/gradeium/backend/internal/database"
)

func TestPhase6DatabaseMigratesToPhase11WithoutRewritingRatings(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	applyMigrationsThrough(t, ctx, testURL, 6)
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(display_name) VALUES('Migration User') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entities(id,type) VALUES
		('0198b0f1-0000-7000-8000-000000000011','game'),
		('0198b0f1-0000-7000-8000-000000000012','movie'),
		('0198b0f1-0000-7000-8000-000000000013','tv_show');
		INSERT INTO games(entity_id,igdb_id,english_title,game_type) VALUES('0198b0f1-0000-7000-8000-000000000011',11,'Game','main_game');
		INSERT INTO movies(entity_id,tmdb_id,english_title) VALUES('0198b0f1-0000-7000-8000-000000000012',12,'Movie');
		INSERT INTO tv_shows(entity_id,tvdb_id,english_title) VALUES('0198b0f1-0000-7000-8000-000000000013',13,'Show')`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO user_games(user_id,game_id,status,rating) VALUES($1,'0198b0f1-0000-7000-8000-000000000011','completed',10)`,
		`INSERT INTO user_movies(user_id,movie_id,status,rating) VALUES($1,'0198b0f1-0000-7000-8000-000000000012','completed',50)`,
		`INSERT INTO user_tv_shows(user_id,tv_show_id,status,rating) VALUES($1,'0198b0f1-0000-7000-8000-000000000013','completed',100)`,
		`INSERT INTO user_settings(user_id) VALUES($1)`,
	} {
		if _, err := pool.Exec(ctx, statement, userID); err != nil {
			t.Fatal(err)
		}
	}
	pool.Close()
	if err := database.Migrate(ctx, testURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate Phase 6 database to 1.1: %v", err)
	}
	pool, err = database.Open(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var gameRating, movieRating, tvRating int
	var scale string
	if err := pool.QueryRow(ctx, `SELECT ug.rating,um.rating,ut.rating,us.rating_scale FROM user_games ug JOIN user_movies um ON um.user_id=ug.user_id JOIN user_tv_shows ut ON ut.user_id=ug.user_id JOIN user_settings us ON us.user_id=ug.user_id WHERE ug.user_id=$1`, userID).Scan(&gameRating, &movieRating, &tvRating, &scale); err != nil {
		t.Fatal(err)
	}
	if gameRating != 10 || movieRating != 50 || tvRating != 100 || scale != "0_10" {
		t.Fatalf("migrated ratings/preferences = (%d, %d, %d, %q)", gameRating, movieRating, tvRating, scale)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_games SET rating=0 WHERE user_id=$1`, userID); err != nil {
		t.Fatalf("canonical zero rating rejected after migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_movies SET rating=-1 WHERE user_id=$1`, userID); err == nil {
		t.Fatal("negative personal rating accepted after migration")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integration_test_status(provider,status,message) VALUES('jellyfin','connected','ok')`); err != nil {
		t.Fatalf("Jellyfin integration status rejected after migration: %v", err)
	}
}
