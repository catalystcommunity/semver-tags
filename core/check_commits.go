package core

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func CheckIfError(err error) {
	if err == nil {
		return
	}

	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}

func Check_commits() {
	repo, err := git.PlainOpen(".")

	fmt.Println(fmt.Sprintf("thing: %v", repo))
	if err == nil {
		ref, headErr := repo.Head()
		if headErr == nil {
			cIter, logErr := repo.Log(&git.LogOptions{From: ref.Hash(), Order: git.LogOrderCommitterTime})
			if logErr == nil {
				iterErr := cIter.ForEach(func(commit *object.Commit) error {
					fmt.Println(commit)

					return nil
				})
				CheckIfError(iterErr)
			}
		}
	}
}
