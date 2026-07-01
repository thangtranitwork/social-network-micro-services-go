package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type loginResponse struct {
	Body struct {
		Token string `json:"token"`
	} `json:"body"`
}

type postResponse struct {
	Body struct {
		ID string `json:"id"`
	} `json:"body"`
}

type newsfeedResponse struct {
	Body []struct {
		ID       string `json:"id"`
		Content  string `json:"content"`
		Username string `json:"username"`
	} `json:"body"`
}

func main() {
	targetURL := flag.String("url", "http://localhost:11111", "API Gateway base URL")
	concurrency := flag.Int("c", 25, "Number of concurrent workers (simulated users)")
	totalCycles := flag.Int("n", 30, "Number of scenario cycles to run per worker")
	delayMs := flag.Int("delay", 100, "Delay in milliseconds between scenario steps")
	timeout := flag.Duration("timeout", 8*time.Second, "HTTP request timeout")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	fmt.Printf("==================================================\n")
	fmt.Printf("   Advanced Multi-Scenario Load Generator (Go)\n")
	fmt.Printf("==================================================\n")
	fmt.Printf("Gateway URL:      %s\n", *targetURL)
	fmt.Printf("Concurrent Users: %d\n", *concurrency)
	fmt.Printf("Cycles/Worker:    %d\n", *totalCycles)
	fmt.Printf("Step Delay:       %d ms\n", *delayMs)
	fmt.Printf("HTTP Timeout:     %v\n", *timeout)
	fmt.Printf("==================================================\n\n")

	// Pre-generate accounts list (using seeded users 1-90 and presidents)
	var testAccounts []struct {
		Email    string
		Username string
	}
	// Add presidents
	presidents := []string{"obama", "trump", "biden", "putin", "xijinping", "macron", "zelenskyy", "hochiminh"}
	for i, p := range presidents {
		testAccounts = append(testAccounts, struct {
			Email    string
			Username string
		}{
			Email:    fmt.Sprintf("test-%d@test.com", i+1),
			Username: p,
		})
	}
	// Add generic test users (user_1 to user_90)
	for i := 1; i <= 90; i++ {
		testAccounts = append(testAccounts, struct {
			Email    string
			Username string
		}{
			Email:    fmt.Sprintf("test-%d@test.com", i+10),
			Username: fmt.Sprintf("user_%d", i),
		})
	}

	var (
		wg                sync.WaitGroup
		successAPIs       int64
		postsCreated      int64
		commentsAdded     int64
		commentsRead      int64
		likesGiven        int64
		chatsSent         int64
		profilesViewed    int64
		searchesDone      int64
		filesTouched      int64
		friendsTouched    int64
		notificationsRead int64
		storiesTouched    int64
		errorsCount       int64
	)

	startTime := time.Now()
	transport := &http.Transport{
		MaxIdleConns:        *concurrency * 8,
		MaxIdleConnsPerHost: *concurrency * 4,
		MaxConnsPerHost:     *concurrency * 8,
		IdleConnTimeout:     90 * time.Second,
	}

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout:   *timeout,
				Transport: transport,
			}

			// Assign a distinct account to this worker
			account := testAccounts[workerID%len(testAccounts)]

			// 1. Authenticate
			loginBody, _ := json.Marshal(map[string]string{
				"email":    account.Email,
				"password": "123456Aa@",
			})
			req, err := http.NewRequest("POST", *targetURL+"/v1/auth/login", bytes.NewBuffer(loginBody))
			if err != nil {
				atomic.AddInt64(&errorsCount, 1)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&errorsCount, 1)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				atomic.AddInt64(&errorsCount, 1)
				return
			}

			var logResp loginResponse
			if err := json.NewDecoder(resp.Body).Decode(&logResp); err != nil {
				atomic.AddInt64(&errorsCount, 1)
				return
			}
			token := logResp.Body.Token
			atomic.AddInt64(&successAPIs, 1)

			// 2. Perform various scenarios in cycles
			for cycle := 0; cycle < *totalCycles; cycle++ {
				// Randomly select one scenario for this cycle
				scenarioType := rand.Intn(8)

				switch scenarioType {
				case 0:
					// --- SCENARIO 0: Content Creator ---
					// A: Create Post
					postText := fmt.Sprintf("Just created a new status update! User @%s testing Go microservices. Timestamp: %s", account.Username, time.Now().Format("15:04:05"))
					postBody, _ := json.Marshal(map[string]interface{}{
						"content": postText,
						"privacy": "PUBLIC",
					})
					req, _ = http.NewRequest("POST", *targetURL+"/v1/posts/post", bytes.NewBuffer(postBody))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+token)
					resp, err = client.Do(req)
					if err != nil {
						atomic.AddInt64(&errorsCount, 1)
					} else {
						bodyBytes, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successAPIs, 1)
							atomic.AddInt64(&postsCreated, 1)

							// Try to self-like
							var pResp postResponse
							if err := json.Unmarshal(bodyBytes, &pResp); err == nil && pResp.Body.ID != "" {
								time.Sleep(time.Duration(*delayMs) * time.Millisecond)
								reqLike, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/posts/like/%s", *targetURL, pResp.Body.ID), nil)
								reqLike.Header.Set("Authorization", "Bearer "+token)
								if respL, errL := client.Do(reqLike); errL == nil {
									respL.Body.Close()
									if respL.StatusCode == http.StatusOK {
										atomic.AddInt64(&successAPIs, 1)
										atomic.AddInt64(&likesGiven, 1)
									} else {
										atomic.AddInt64(&errorsCount, 1)
									}
								} else {
									atomic.AddInt64(&errorsCount, 1)
								}
							}
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}

				case 1:
					// --- SCENARIO 1: Feeder & Commenter ---
					// A: Fetch Newsfeed
					req, _ = http.NewRequest("GET", *targetURL+"/v1/posts/newsfeed", nil)
					req.Header.Set("Authorization", "Bearer "+token)
					resp, err = client.Do(req)
					if err != nil {
						atomic.AddInt64(&errorsCount, 1)
					} else {
						bodyBytes, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successAPIs, 1)

							var feed newsfeedResponse
							if err := json.Unmarshal(bodyBytes, &feed); err == nil && len(feed.Body) > 0 {
								// Interact with a random post from newsfeed
								targetPost := feed.Body[rand.Intn(len(feed.Body))]

								time.Sleep(time.Duration(*delayMs) * time.Millisecond)

								// B: Comment on it
								commentBody, _ := json.Marshal(map[string]interface{}{
									"postId":  targetPost.ID,
									"content": fmt.Sprintf("Interesting thoughts @%s! Greetings from @%s.", targetPost.Username, account.Username),
								})
								reqComment, _ := http.NewRequest("POST", *targetURL+"/v1/posts/comment", bytes.NewBuffer(commentBody))
								reqComment.Header.Set("Content-Type", "application/json")
								reqComment.Header.Set("Authorization", "Bearer "+token)
								if respC, errC := client.Do(reqComment); errC == nil {
									respC.Body.Close()
									if respC.StatusCode == http.StatusOK {
										atomic.AddInt64(&successAPIs, 1)
										atomic.AddInt64(&commentsAdded, 1)
									} else {
										atomic.AddInt64(&errorsCount, 1)
									}
								} else {
									atomic.AddInt64(&errorsCount, 1)
								}

								// C: Like the post
								time.Sleep(time.Duration(*delayMs) * time.Millisecond)
								reqL, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/posts/like/%s", *targetURL, targetPost.ID), nil)
								reqL.Header.Set("Authorization", "Bearer "+token)
								if respL, errL := client.Do(reqL); errL == nil {
									respL.Body.Close()
									if respL.StatusCode == http.StatusOK {
										atomic.AddInt64(&successAPIs, 1)
										atomic.AddInt64(&likesGiven, 1)
									}
								}
							}
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}

				case 2:
					// --- SCENARIO 2: Chat Room active talker ---
					// A: Fetch Chat List
					req, _ = http.NewRequest("GET", *targetURL+"/v1/chat", nil)
					req.Header.Set("Authorization", "Bearer "+token)
					resp, err = client.Do(req)
					if err != nil {
						atomic.AddInt64(&errorsCount, 1)
					} else {
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successAPIs, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}

					time.Sleep(time.Duration(*delayMs) * time.Millisecond)

					// B: Send Chat Message to a random partner
					partner := testAccounts[rand.Intn(len(testAccounts))]
					if partner.Username != account.Username {
						chatBody, _ := json.Marshal(map[string]string{
							"username": partner.Username,
							"text":     fmt.Sprintf("Hey @%s, how are you? Let's catch up sometime! Sent from @%s.", partner.Username, account.Username),
						})
						reqChat, _ := http.NewRequest("POST", *targetURL+"/v1/chat/send", bytes.NewBuffer(chatBody))
						reqChat.Header.Set("Content-Type", "application/json")
						reqChat.Header.Set("Authorization", "Bearer "+token)
						if respChat, errChat := client.Do(reqChat); errChat == nil {
							respChat.Body.Close()
							if respChat.StatusCode == http.StatusOK {
								atomic.AddInt64(&successAPIs, 1)
								atomic.AddInt64(&chatsSent, 1)
							} else {
								atomic.AddInt64(&errorsCount, 1)
							}
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}

				case 3:
					// --- SCENARIO 3: Investigator / Profiler stalker ---
					// A: Search Users
					targetUser := testAccounts[rand.Intn(len(testAccounts))].Username
					req, _ = http.NewRequest("GET", *targetURL+"/v1/search?query="+targetUser, nil)
					req.Header.Set("Authorization", "Bearer "+token)
					resp, err = client.Do(req)
					if err != nil {
						atomic.AddInt64(&errorsCount, 1)
					} else {
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successAPIs, 1)
							atomic.AddInt64(&searchesDone, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}

					time.Sleep(time.Duration(*delayMs) * time.Millisecond)

					// B: View Profile
					reqProf, _ := http.NewRequest("GET", *targetURL+"/v1/users/"+targetUser, nil)
					reqProf.Header.Set("Authorization", "Bearer "+token)
					if respP, errP := client.Do(reqProf); errP == nil {
						_, _ = io.Copy(io.Discard, respP.Body)
						respP.Body.Close()
						if respP.StatusCode == http.StatusOK {
							atomic.AddInt64(&successAPIs, 1)
							atomic.AddInt64(&profilesViewed, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					} else {
						atomic.AddInt64(&errorsCount, 1)
					}

				case 4:
					// --- SCENARIO 4: File and media browser ---
					targetUser := testAccounts[rand.Intn(len(testAccounts))].Username
					params := url.Values{}
					params.Set("filename", fmt.Sprintf("load-test-%d-%d.png", workerID, cycle))
					params.Set("contentType", "image/png")

					if status, err := doAuthenticated(client, "GET", *targetURL+"/v1/files/upload/presigned?"+params.Encode(), token, nil, ""); err == nil && isSuccess(status) {
						atomic.AddInt64(&successAPIs, 1)
						atomic.AddInt64(&filesTouched, 1)
					} else {
						atomic.AddInt64(&errorsCount, 1)
					}

					time.Sleep(time.Duration(*delayMs) * time.Millisecond)

					filesURL := fmt.Sprintf("%s/v1/posts/files/%s?skip=%d&limit=%d", *targetURL, url.PathEscape(targetUser), rand.Intn(3), 5+rand.Intn(10))
					if status, err := doAuthenticated(client, "GET", filesURL, token, nil, ""); err == nil && isSuccess(status) {
						atomic.AddInt64(&successAPIs, 1)
						atomic.AddInt64(&filesTouched, 1)
					} else {
						atomic.AddInt64(&errorsCount, 1)
					}

				case 5:
					// --- SCENARIO 5: Social graph crawler ---
					targetUser := testAccounts[rand.Intn(len(testAccounts))].Username
					endpoints := []string{
						"/v1/friends/suggested",
						"/v1/friends/" + url.PathEscape(targetUser),
						"/v1/friends/mutual-friends/" + url.PathEscape(targetUser),
						"/v1/blocks",
					}

					for _, endpoint := range endpoints {
						if status, err := doAuthenticated(client, "GET", *targetURL+endpoint, token, nil, ""); err == nil && isSuccess(status) {
							atomic.AddInt64(&successAPIs, 1)
							atomic.AddInt64(&friendsTouched, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
						if *delayMs > 0 {
							time.Sleep(time.Duration(*delayMs) * time.Millisecond)
						}
					}

				case 6:
					// --- SCENARIO 6: Notifications reader ---
					endpoints := []string{
						"/v1/notifications?skip=0&limit=20",
						"/v1/notifications/unread-count",
					}

					for _, endpoint := range endpoints {
						if status, err := doAuthenticated(client, "GET", *targetURL+endpoint, token, nil, ""); err == nil && isSuccess(status) {
							atomic.AddInt64(&successAPIs, 1)
							atomic.AddInt64(&notificationsRead, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
						if *delayMs > 0 {
							time.Sleep(time.Duration(*delayMs) * time.Millisecond)
						}
					}

				case 7:
					// --- SCENARIO 7: Stories and comments explorer ---
					if status, err := doAuthenticated(client, "GET", *targetURL+"/v1/stories/feed", token, nil, ""); err == nil && isSuccess(status) {
						atomic.AddInt64(&successAPIs, 1)
						atomic.AddInt64(&storiesTouched, 1)
					} else {
						atomic.AddInt64(&errorsCount, 1)
					}

					time.Sleep(time.Duration(*delayMs) * time.Millisecond)

					reqFeed, _ := http.NewRequest("GET", *targetURL+"/v1/posts/newsfeed?skip=0&limit=10", nil)
					reqFeed.Header.Set("Authorization", "Bearer "+token)
					if respFeed, errFeed := client.Do(reqFeed); errFeed == nil {
						bodyBytes, _ := io.ReadAll(respFeed.Body)
						respFeed.Body.Close()
						if isSuccess(respFeed.StatusCode) {
							atomic.AddInt64(&successAPIs, 1)
							var feed newsfeedResponse
							if err := json.Unmarshal(bodyBytes, &feed); err == nil && len(feed.Body) > 0 {
								targetPost := feed.Body[rand.Intn(len(feed.Body))]
								commentsURL := fmt.Sprintf("%s/v1/comments/of-post/%s?skip=0&limit=10", *targetURL, url.PathEscape(targetPost.ID))
								if status, err := doAuthenticated(client, "GET", commentsURL, token, nil, ""); err == nil && isSuccess(status) {
									atomic.AddInt64(&successAPIs, 1)
									atomic.AddInt64(&commentsRead, 1)
								} else {
									atomic.AddInt64(&errorsCount, 1)
								}
							}
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					} else {
						atomic.AddInt64(&errorsCount, 1)
					}
				}

				if *delayMs > 0 {
					time.Sleep(time.Duration(*delayMs) * time.Millisecond)
				}
			}
		}(i)
	}

	// Reporter
	stopReporter := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := atomic.LoadInt64(&successAPIs)
				pc := atomic.LoadInt64(&postsCreated)
				cc := atomic.LoadInt64(&commentsAdded)
				cr := atomic.LoadInt64(&commentsRead)
				lg := atomic.LoadInt64(&likesGiven)
				cs := atomic.LoadInt64(&chatsSent)
				pv := atomic.LoadInt64(&profilesViewed)
				sd := atomic.LoadInt64(&searchesDone)
				ft := atomic.LoadInt64(&filesTouched)
				fg := atomic.LoadInt64(&friendsTouched)
				nr := atomic.LoadInt64(&notificationsRead)
				st := atomic.LoadInt64(&storiesTouched)
				e := atomic.LoadInt64(&errorsCount)
				fmt.Printf("\rAPIs: %d | Posts: %d | Comments+: %d | CommentsR: %d | Likes: %d | Chats: %d | Profile: %d | Search: %d | Files: %d | Friends: %d | Notifs: %d | Stories: %d | Err: %d",
					s, pc, cc, cr, lg, cs, pv, sd, ft, fg, nr, st, e)
			case <-stopReporter:
				return
			}
		}
	}()

	wg.Wait()
	close(stopReporter)

	duration := time.Since(startTime)
	fmt.Printf("\n\n================- SIMULATION RESULTS -================\n")
	fmt.Printf("Duration:                   %v\n", duration)
	fmt.Printf("Successful API Calls:       %d\n", successAPIs)
	fmt.Printf("Posts Created:              %d\n", postsCreated)
	fmt.Printf("Comments Added:             %d\n", commentsAdded)
	fmt.Printf("Comments Read:              %d\n", commentsRead)
	fmt.Printf("Likes Given:                %d\n", likesGiven)
	fmt.Printf("Chat Messages Sent:         %d\n", chatsSent)
	fmt.Printf("Profiles Viewed:            %d\n", profilesViewed)
	fmt.Printf("Searches Conducted:         %d\n", searchesDone)
	fmt.Printf("File APIs Touched:          %d\n", filesTouched)
	fmt.Printf("Friend APIs Touched:        %d\n", friendsTouched)
	fmt.Printf("Notifications Read:         %d\n", notificationsRead)
	fmt.Printf("Stories Touched:            %d\n", storiesTouched)
	fmt.Printf("Errors/Failures:            %d\n", errorsCount)
	fmt.Printf("Total Successful Ops:       %d\n", successAPIs)
	fmt.Printf("======================================================\n")
}

func doAuthenticated(client *http.Client, method, rawURL, token string, body io.Reader, contentType string) (int, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func isSuccess(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}
