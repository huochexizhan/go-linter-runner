package main

import (
	"bufio"
	"fmt"
	"os"
)

// 我有两个文件，realtop1000.txt和star1000-4000.txt，各有1000行，帮我用go写一段代码，打印出仅star1000-4000.txt中有，而realtop1000.txt中没有的内容

func main() {
	// 打开文件
	realtopFile, err := os.Open("realtop1000.txt")
	if err != nil {
		fmt.Println("Error opening realtop1000.txt:", err)
		return
	}
	defer realtopFile.Close()

	starFile, err := os.Open("star1000-4000.txt")
	if err != nil {
		fmt.Println("Error opening star1000-4000.txt:", err)
		return
	}
	defer starFile.Close()

	// 读取 realtop1000.txt 内容到一个 map 中
	realtopSet := make(map[string]struct{})
	scanner := bufio.NewScanner(realtopFile)
	for scanner.Scan() {
		line := scanner.Text()
		realtopSet[line] = struct{}{} // 使用空结构体作为值
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading realtop1000.txt:", err)
		return
	}

	// 检查 star1000-4000.txt 中的内容
	scanner = bufio.NewScanner(starFile)
	for scanner.Scan() {
		line := scanner.Text()
		if _, found := realtopSet[line]; !found {
			// 如果在 realtopSet 中找不到，则打印该行
			fmt.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading star1000-4000.txt:", err)
		return
	}
}
