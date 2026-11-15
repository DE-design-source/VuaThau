package tests

import (
	"context"
	"sort"
	"sync"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// memStore is a shared in-memory backing store for the fake repositories.
type memStore struct {
	mu         sync.Mutex
	users      map[int64]model.User
	messages   []model.Message
	nextUserID int64
	nextMsgID  int64
}

func newMemStore() *memStore {
	return &memStore{users: make(map[int64]model.User)}
}

// Compile-time interface checks.
var (
	_ repository.AuthRepository    = (*fakeAuthRepo)(nil)
	_ repository.UserRepository    = (*fakeUserRepo)(nil)
	_ repository.MessageRepository = (*fakeMessageRepo)(nil)
)

// ---- AuthRepository ----

type fakeAuthRepo struct{ s *memStore }

func (r *fakeAuthRepo) CreateUser(_ context.Context, u *model.User) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for _, existing := range r.s.users {
		if existing.Email == u.Email || existing.Username == u.Username {
			return repository.ErrDuplicate
		}
	}
	r.s.nextUserID++
	now := time.Now().UTC()
	u.ID = r.s.nextUserID
	u.CreatedAt = now
	u.UpdatedAt = now
	r.s.users[u.ID] = *u
	return nil
}

func (r *fakeAuthRepo) FindByEmail(_ context.Context, email string) (*model.User, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for _, u := range r.s.users {
		if u.Email == email {
			cp := u
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}

// ---- UserRepository ----

type fakeUserRepo struct{ s *memStore }

// GetByID mirrors the SQL implementation, which does not select password_hash.
func (r *fakeUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	u, ok := r.s.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	u.PasswordHash = ""
	return &u, nil
}

func (r *fakeUserRepo) List(_ context.Context, limit, offset int) ([]model.User, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	all := make([]model.User, 0, len(r.s.users))
	for _, u := range r.s.users {
		u.PasswordHash = ""
		all = append(all, u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return paginate(all, limit, offset), nil
}

// ---- MessageRepository ----

type fakeMessageRepo struct{ s *memStore }

func (r *fakeMessageRepo) Create(_ context.Context, m *model.Message) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	// Mirror the FK constraints: both sender and receiver must exist.
	if _, ok := r.s.users[m.ReceiverID]; !ok {
		return repository.ErrNotFound
	}
	if _, ok := r.s.users[m.SenderID]; !ok {
		return repository.ErrNotFound
	}
	r.s.nextMsgID++
	m.ID = r.s.nextMsgID
	m.CreatedAt = time.Now().UTC()
	r.s.messages = append(r.s.messages, *m)
	return nil
}

func (r *fakeMessageRepo) ListConversation(_ context.Context, userA, userB int64, limit, offset int) ([]model.Message, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var conv []model.Message
	// Iterate backwards: messages are appended in id order, so this is newest first
	// (matches ORDER BY created_at DESC, id DESC).
	for i := len(r.s.messages) - 1; i >= 0; i-- {
		m := r.s.messages[i]
		if (m.SenderID == userA && m.ReceiverID == userB) || (m.SenderID == userB && m.ReceiverID == userA) {
			conv = append(conv, m)
		}
	}
	return paginate(conv, limit, offset), nil
}

func paginate[T any](items []T, limit, offset int) []T {
	out := make([]T, 0)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return out
	}
	end := len(items)
	if limit >= 0 && offset+limit < end {
		end = offset + limit
	}
	return append(out, items[offset:end]...)
}
