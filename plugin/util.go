// Copyright 2020 the Drone Authors. All rights reserved.
// Use of this source code is governed by the Blue Oak Model License
// that can be found in the LICENSE file.

package plugin

import (
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
)

func GetNewError(s string) error {
	return errors.New(s)
}

func LogPrintln(p Plugin, args ...interface{}) {

	if p != nil {
		if p.IsQuiet() {
			return
		}
	}

	logrus.Println(append([]interface{}{"Plugin Info:"}, args...)...)
}

func GetOutputVariablesStorageFilePath() string {
	return os.Getenv("DRONE_OUTPUT")
}

func WriteEnvVariableAsString(key string, value interface{}) error {

	if GetOutputVariablesStorageFilePath() == "" {
		logrus.Println("Output file path is empty, check env var DRONE_OUTPUT")
		return nil
	}

	outputFile, err := os.OpenFile(GetOutputVariablesStorageFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer outputFile.Close()

	valueStr := fmt.Sprintf("%v", value)

	_, err = fmt.Fprintf(outputFile, "%s=%s\n", key, valueStr)
	if err != nil {
		return fmt.Errorf("failed to write to env: %w", err)
	}

	return nil
}
