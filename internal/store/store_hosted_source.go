package store

import "github.com/eyupio/reet.sh/internal/skillbundle"

// PublishedHostedFiles is the files of the published skill username/slug as its
// page serves them, or ErrNotFound when it is gone, unpublished or its author
// is disabled. A skill someone else published is copied from here.
func (s *Store) PublishedHostedFiles(username, slug string) ([]skillbundle.ContentFile, error) {
	sk, err := s.PublishedHostedSkill(username, slug)
	if err != nil {
		return nil, err
	}
	return hostedContentFiles(sk), nil
}
