package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/store"
)

var (
	ocUserID       string
	ocNickname     string
	ocDefaultModel string
	ocIsDefault    bool
)

var addOpenClawAccountCmd = &cobra.Command{
	Use:   "add-openclaw-account",
	Short: "注册一个 OpenClaw 账号（自动授权，无需凭据）",
	Long: `注册一个 provider=openclaw 的账号。授权走本机京ME桌面端（HiOffice）自动换取
me_token（约24h，自动续期），无需任何凭据；京ME桌面端在运行即可。

示例：
  JoyCode2Api add-openclaw-account --user-id my-erp --default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if ocUserID == "" {
			return fmt.Errorf("--user-id is required")
		}

		s, err := store.Open("")
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		if err := s.AddOpenClawAccount(ocUserID, ocNickname, ocDefaultModel, ocIsDefault); err != nil {
			return fmt.Errorf("add openclaw account: %w", err)
		}
		model := ocDefaultModel
		if model == "" {
			model = "JoyAI"
		}
		fmt.Printf("✓ OpenClaw account %q registered (model=%s)\n", ocUserID, model)
		return nil
	},
}

func init() {
	addOpenClawAccountCmd.Flags().StringVar(&ocUserID, "user-id", "", "账号 user_id（任意标识）")
	addOpenClawAccountCmd.Flags().StringVar(&ocNickname, "nickname", "OpenClaw", "账号昵称")
	addOpenClawAccountCmd.Flags().StringVar(&ocDefaultModel, "model", "JoyAI", "默认模型名（JoyAI / Dr.Joy）")
	addOpenClawAccountCmd.Flags().BoolVar(&ocIsDefault, "default", false, "设为默认账号")
	rootCmd.AddCommand(addOpenClawAccountCmd)
}
