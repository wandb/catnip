package cmd

import (
	"fmt"
	goruntime "runtime"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/services"
)

var downloadCmd = &cobra.Command{
	Use:    "download",
	Short:  "📦 Download inference dependencies",
	Hidden: true,
	Long: `Download llama.cpp libraries and GGUF model for local inference.

This command downloads:
- llama.cpp libraries for your platform (stored in ~/.catnip/lib)
- Gemma 270M summarizer model (stored in ~/.catnip/models)

After running this command, inference will work offline without any additional downloads.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDownload(cmd)
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Add flags
	downloadCmd.Flags().Bool("libraries-only", false, "Download only llama.cpp libraries")
	downloadCmd.Flags().Bool("model-only", false, "Download only the GGUF model")
	downloadCmd.Flags().Bool("force", false, "Force re-download even if files exist")
}

func runDownload(cmd *cobra.Command) error {
	librariesOnly, _ := cmd.Flags().GetBool("libraries-only")
	modelOnly, _ := cmd.Flags().GetBool("model-only")
	force, _ := cmd.Flags().GetBool("force")

	// Configure logging
	logger.Configure(logger.LevelInfo, true)

	// Determine what to download
	downloadLibraries := !modelOnly
	downloadModel := !librariesOnly

	// Check platform support
	if downloadLibraries {
		if goruntime.GOOS != "darwin" && goruntime.GOOS != "linux" && goruntime.GOOS != "windows" {
			logger.Warnf("⚠️  Inference not supported on %s, skipping library download", goruntime.GOOS)
			downloadLibraries = false
		}
	}

	var libPath string
	var modelPath string

	// Download libraries
	if downloadLibraries {
		logger.Infof("📚 Downloading llama.cpp libraries for %s/%s...", goruntime.GOOS, goruntime.GOARCH)

		downloader, err := services.NewLibraryDownloader()
		if err != nil {
			return fmt.Errorf("failed to create library downloader: %w", err)
		}

		// Check if library exists
		existingPath, _ := downloader.GetLibraryPath()
		if existingPath != "" && !force {
			logger.Infof("✅ Libraries already downloaded at: %s", existingPath)
			logger.Infof("   Use --force to re-download")
			libPath = existingPath
		} else {
			path, err := downloader.DownloadLibrary()
			if err != nil {
				return fmt.Errorf("failed to download libraries: %w", err)
			}
			libPath = path
			logger.Infof("✅ Libraries installed at: %s", libPath)
		}
	}

	// Download model
	if downloadModel {
		logger.Infof("📦 Downloading GGUF model (Gemma 270M summarizer)...")

		downloader, err := services.NewModelDownloader()
		if err != nil {
			return fmt.Errorf("failed to create model downloader: %w", err)
		}

		modelFilename := "gemma3-270m-summarizer-Q4_K_M.gguf"
		modelURL := "https://huggingface.co/vanpelt/catnip-summarizer/resolve/main/gemma3-270m-summarizer-Q4_K_M.gguf"

		// Check if model exists
		existingModelPath := downloader.GetModelPath(modelFilename)
		if !force {
			// Check if file exists and has reasonable size (> 100MB)
			if info, err := services.StatFile(existingModelPath); err == nil && info.Size() > 100*1024*1024 {
				logger.Infof("✅ Model already downloaded at: %s", existingModelPath)
				logger.Infof("   Size: %.1f MB", float64(info.Size())/(1024*1024))
				logger.Infof("   Use --force to re-download")
				modelPath = existingModelPath
			} else {
				// Model doesn't exist or is incomplete, download it
				path, err := downloader.DownloadModel(modelURL, modelFilename, "")
				if err != nil {
					return fmt.Errorf("failed to download model: %w", err)
				}
				modelPath = path

				// Get file size for confirmation
				if info, err := services.StatFile(modelPath); err == nil {
					logger.Infof("✅ Model installed at: %s", modelPath)
					logger.Infof("   Size: %.1f MB", float64(info.Size())/(1024*1024))
				}
			}
		} else {
			// Force download
			path, err := downloader.DownloadModel(modelURL, modelFilename, "")
			if err != nil {
				return fmt.Errorf("failed to download model: %w", err)
			}
			modelPath = path

			// Get file size for confirmation
			if info, err := services.StatFile(modelPath); err == nil {
				logger.Infof("✅ Model installed at: %s", modelPath)
				logger.Infof("   Size: %.1f MB", float64(info.Size())/(1024*1024))
			}
		}
	}

	// Print summary
	fmt.Println()
	logger.Infof("🎉 Download complete!")
	if downloadLibraries && libPath != "" {
		logger.Infof("   Libraries: %s", libPath)
	}
	if downloadModel && modelPath != "" {
		logger.Infof("   Model: %s", modelPath)
	}
	fmt.Println()
	logger.Infof("💡 You can now use inference offline with 'catnip serve'")

	return nil
}
