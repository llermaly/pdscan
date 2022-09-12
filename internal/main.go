package internal

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

func Main(urlStr string, showData bool, showAll bool, limit int, processes int) error {
	runtime.GOMAXPROCS(processes)

	var matchList []ruleMatch
	var err error

	if strings.HasPrefix(urlStr, "file://") || strings.HasPrefix(urlStr, "s3://") {
		var adapter FileAdapter
		if strings.HasPrefix(urlStr, "file://") {
			adapter = &LocalFileAdapter{}
		} else {
			adapter = &S3Adapter{}
		}

		matchList, err = fileAdapterGo(&adapter, urlStr, showData, showAll)
	} else {
		var adapter DataStoreAdapter
		if strings.HasPrefix(urlStr, "mongodb://") {
			adapter = &MongodbAdapter{}
		} else if strings.HasPrefix(urlStr, "redis://") {
			adapter = &RedisAdapter{}
		} else if strings.HasPrefix(urlStr, "elasticsearch+http://") || strings.HasPrefix(urlStr, "elasticsearch+https://") || strings.HasPrefix(urlStr, "opensearch+http://") || strings.HasPrefix(urlStr, "opensearch+https://") {
			adapter = &ElasticsearchAdapter{}
		} else {
			adapter = &SqlAdapter{}
		}

		matchList, err = dataStoreAdapterGo(&adapter, urlStr, showData, showAll, limit)
	}

	if err != nil {
		return err
	}

	if matchList == nil {
		return nil
	}

	if len(matchList) > 0 {
		if showData {
			fmt.Println("Showing 50 unique values from each")
		} else {
			fmt.Println("\nUse --show-data to view data")
		}

		if !showAll {
			showLowConfidenceMatchHelp(matchList)
		}
	} else {
		fmt.Println("No sensitive data found")
	}

	return nil
}

func dataStoreAdapterGo(a *DataStoreAdapter, urlStr string, showData bool, showAll bool, limit int) ([]ruleMatch, error) {
	adapter := *a

	err := adapter.Init(urlStr)
	if err != nil {
		return nil, err
	}

	tables, err := adapter.FetchTables()
	if err != nil {
		return nil, err
	}

	if len(tables) > 0 {
		fmt.Printf("Found %s to scan, sampling %s from each...\n\n", pluralize(len(tables), adapter.TableName()), pluralize(limit, adapter.RowName()))

		matchList := []ruleMatch{}

		var g errgroup.Group
		var appendMutex sync.Mutex
		var queryMutex sync.Mutex

		for _, table := range tables {
			// important - do not remove
			// https://go.dev/doc/faq#closures_and_goroutines
			table := table

			g.Go(func() error {
				queryMutex.Lock()
				columnNames, columnValues, err := adapter.FetchTableData(table, limit)
				queryMutex.Unlock()
				if err != nil {
					return err
				}

				tableMatchList := checkTableData(table, columnNames, columnValues)
				printMatchList(tableMatchList, showData, showAll, adapter.RowName())

				appendMutex.Lock()
				matchList = append(matchList, tableMatchList...)
				appendMutex.Unlock()

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}

		return matchList, nil
	} else {
		fmt.Printf("Found no %s to scan\n", pluralize(0, adapter.TableName())[2:])
		return nil, nil
	}
}

func fileAdapterGo(a *FileAdapter, urlStr string, showData bool, showAll bool) ([]ruleMatch, error) {
	adapter := *a

	err := adapter.Init(urlStr)
	if err != nil {
		return nil, err
	}

	files, err := adapter.FetchFiles()
	if err != nil {
		return nil, err
	}

	if len(files) > 0 {
		fmt.Printf("Found %s to scan...\n\n", pluralize(len(files), "file"))

		matchList := []ruleMatch{}

		var g errgroup.Group
		var appendMutex sync.Mutex

		g.SetLimit(20)

		for _, file := range files {
			// important - do not remove
			// https://go.dev/doc/faq#closures_and_goroutines
			file := file

			g.Go(func() error {
				// fmt.Println("Scanning " + file + "...\n")
				matchedValues, count, err := adapter.FindFileMatches(file)
				if err != nil {
					return err
				}
				fileMatchList := checkMatches(file, matchedValues, count, true)
				printMatchList(fileMatchList, showData, showAll, "line")

				appendMutex.Lock()
				matchList = append(matchList, fileMatchList...)
				appendMutex.Unlock()

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}

		return matchList, nil
	} else {
		fmt.Println("Found no files to scan")
		return nil, nil
	}
}
