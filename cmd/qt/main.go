package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"exceltranslator/pkg/config"
	"exceltranslator/pkg/runner"
)

//go:embed icon.png
var appIconData []byte

// MainWindow Excel翻译器的主窗口，包含所有UI组件和状态管理
type MainWindow struct {
	window *qt.QMainWindow

	// 文件操作相关UI组件
	inputFileEdit *qt.QLineEdit   // 输入文件路径显示框
	inputFileBtn  *qt.QPushButton // 文件浏览按钮
	fileGroup     *qt.QGroupBox   // 文件选择区域容器，支持拖拽

	// 设置页面UI组件
	apiKeyEdit            *qt.QLineEdit // API密钥输入框
	apiUrlEdit            *qt.QLineEdit // API地址输入框
	modelEdit             *qt.QLineEdit // 模型名称输入框
	promptEdit            *qt.QTextEdit // 翻译提示词输入框
	maxConcurrentSpin     *qt.QSpinBox  // 最大并发数设置
	onlyTranslateCJKCheck *qt.QCheckBox // 仅翻译CJK文本选项

	// 主界面控制组件
	progressBar *qt.QProgressBar // 翻译进度条
	logTextEdit *qt.QTextEdit    // 日志显示区域
	startBtn    *qt.QPushButton  // 开始翻译按钮
	stopBtn     *qt.QPushButton  // 停止翻译按钮

	// 应用状态
	isTranslating  bool   // 当前是否正在翻译
	tempOutputFile string // 临时输出文件路径
	lastOpenDir    string // 上次打开文件的目录
	lastSaveDir    string // 上次保存文件的目录

	// 协程控制
	ctx    context.Context    // 用于取消翻译操作的上下文
	cancel context.CancelFunc // 取消函数

	// 状态保护
	stateMutex sync.Mutex // 保护翻译状态的互斥锁
}

// NewMainWindow 创建主窗口实例，初始化所有UI组件和布局
func NewMainWindow() *MainWindow {
	mw := &MainWindow{}

	mw.window = qt.NewQMainWindow2()
	mw.window.SetWindowTitle("Excel 翻译器")
	mw.window.SetMinimumSize(qt.NewQSize2(600, 400))
	mw.window.Resize(800, 400)

	if runtime.GOOS == "darwin" {
		mw.createMenuBar()
	}

	centralWidget := qt.NewQWidget2()
	mw.window.SetCentralWidget(centralWidget)

	mainLayout := qt.NewQVBoxLayout2()
	mainLayout.SetSpacing(20)
	mainLayout.SetContentsMargins(25, 25, 25, 25)
	centralWidget.SetLayout(mainLayout.QBoxLayout.QLayout)

	translationTab := mw.createTranslationPage()
	mainLayout.AddWidget(translationTab)

	mw.setupDragAndDrop()

	return mw
}

// createTranslationPage 创建翻译页面，包含文件选择区域、进度条、控制按钮和日志显示
func (mw *MainWindow) createTranslationPage() *qt.QWidget {
	page := qt.NewQWidget2()
	mainLayout := qt.NewQHBoxLayout2()
	page.SetLayout(mainLayout.QBoxLayout.QLayout)

	leftGroup := qt.NewQGroupBox(page)
	leftGroup.SetFixedWidth(350)
	leftLayout := qt.NewQVBoxLayout2()
	leftLayout.SetSpacing(25)
	leftLayout.SetContentsMargins(15, 15, 15, 15)
	leftGroup.SetLayout(leftLayout.QBoxLayout.QLayout)

	fileGroup := qt.NewQGroupBox(leftGroup.QWidget)
	fileGroup.SetContentsMargins(15, 15, 15, 15)
	fileGroup.SetAcceptDrops(true)
	fileLayout := qt.NewQVBoxLayout2()
	fileLayout.SetSpacing(10)
	fileGroup.SetLayout(fileLayout.QBoxLayout.QLayout)

	fileHint := qt.NewQLabel5("拖拽Excel文件到此区域或点击浏览文件", fileGroup.QWidget)
	fileHint.SetAlignment(qt.AlignCenter)
	fileLayout.AddWidget(fileHint.QWidget)

	mw.inputFileEdit = qt.NewQLineEdit(fileGroup.QWidget)
	mw.inputFileEdit.SetPlaceholderText("选择要翻译的Excel文件...")
	mw.inputFileEdit.SetAcceptDrops(true)
	mw.inputFileEdit.SetReadOnly(true)
	fileLayout.AddWidget(mw.inputFileEdit.QWidget)

	mw.inputFileBtn = qt.NewQPushButton5("浏览文件...", fileGroup.QWidget)
	mw.inputFileBtn.OnPressed(func() {
		mw.selectInputFile()
	})
	fileLayout.AddWidget(mw.inputFileBtn.QWidget)

	leftLayout.AddWidget(fileGroup.QWidget)
	mw.fileGroup = fileGroup

	mw.progressBar = qt.NewQProgressBar(leftGroup.QWidget)
	mw.progressBar.SetRange(0, 100)
	mw.progressBar.SetValue(0)
	mw.progressBar.SetTextVisible(false)
	mw.progressBar.SetFixedHeight(8)
	leftLayout.AddWidget(mw.progressBar.QWidget)

	buttonLayout := qt.NewQHBoxLayout2()
	buttonLayout.SetSpacing(20)
	buttonLayout.AddStretch()

	mw.startBtn = qt.NewQPushButton5("开始翻译", leftGroup.QWidget)
	mw.startBtn.SetFixedWidth(80)
	mw.startBtn.OnPressed(func() {
		mw.startTranslation()
	})
	buttonLayout.AddWidget(mw.startBtn.QWidget)

	buttonLayout.AddSpacing(20)

	mw.stopBtn = qt.NewQPushButton5("停止翻译", leftGroup.QWidget)
	mw.stopBtn.SetFixedWidth(80)
	mw.stopBtn.SetEnabled(false)
	mw.stopBtn.OnPressed(func() {
		mw.stopTranslation()
	})
	buttonLayout.AddWidget(mw.stopBtn.QWidget)
	buttonLayout.AddStretch()

	leftLayout.AddLayout(buttonLayout.QBoxLayout.QLayout)
	leftLayout.AddStretch()

	rightGroup := qt.NewQGroupBox4("日志", page)
	if runtime.GOOS == "darwin" {
		rightGroup.SetStyleSheet(`
QGroupBox::title {
	subcontrol-origin: margin;
	subcontrol-position: top left;
	top: 10px;
	left: 12px;
}
`)
	}
	rightLayout := qt.NewQVBoxLayout2()
	rightLayout.SetContentsMargins(4, 8, 4, 8)
	rightLayout.SetSpacing(2)
	rightGroup.SetLayout(rightLayout.QBoxLayout.QLayout)

	mw.logTextEdit = qt.NewQTextEdit4("", rightGroup.QWidget)
	mw.logTextEdit.SetReadOnly(true)
	mw.logTextEdit.SetContentsMargins(0, 0, 0, 0)
	mw.logTextEdit.SetStyleSheet(`
QTextEdit {
	background-color: transparent;
	box-shadow: none;
	border: none;
	padding: 0px;
}
`)
	rightLayout.AddWidget(mw.logTextEdit.QWidget)

	mainLayout.AddWidget2(leftGroup.QWidget, 0)
	mainLayout.AddWidget2(rightGroup.QWidget, 1)

	return page
}

// createSettingsPage 创建设置页面，包含LLM配置和客户端配置两个分组
func (mw *MainWindow) createSettingsPage() *qt.QWidget {
	settingsPage := qt.NewQWidget2()
	mainLayout := qt.NewQVBoxLayout2()
	mainLayout.SetSpacing(20)
	settingsPage.SetLayout(mainLayout.QBoxLayout.QLayout)

	llmGroup := qt.NewQGroupBox4("LLM 配置", settingsPage)
	llmGroup.SetStyleSheet(`
QGroupBox::title {
	subcontrol-origin: margin;
	subcontrol-position: top left;
	top: 10px;
	left: 12px;
}
`)
	llmLayout := qt.NewQFormLayout2()
	llmLayout.SetContentsMargins(10, 20, 10, 20)
	llmLayout.SetSpacing(15)
	llmLayout.SetLabelAlignment(qt.AlignRight)
	llmLayout.SetFieldGrowthPolicy(qt.QFormLayout__ExpandingFieldsGrow)
	llmGroup.SetLayout(llmLayout.QLayout)

	mw.apiKeyEdit = qt.NewQLineEdit(llmGroup.QWidget)
	mw.apiKeyEdit.SetEchoMode(qt.QLineEdit__Password)
	llmLayout.AddRow3("API Key:", mw.apiKeyEdit.QWidget)

	mw.apiUrlEdit = qt.NewQLineEdit(llmGroup.QWidget)
	llmLayout.AddRow3("API URL:", mw.apiUrlEdit.QWidget)

	mw.modelEdit = qt.NewQLineEdit(llmGroup.QWidget)
	llmLayout.AddRow3("模型:", mw.modelEdit.QWidget)

	mainLayout.AddWidget(llmGroup.QWidget)

	mainLayout.AddSpacing(12)

	clientGroup := qt.NewQGroupBox4("客户端配置", settingsPage)
	clientGroup.SetStyleSheet(`
QGroupBox::title {
	subcontrol-origin: margin;
	subcontrol-position: top left;
	top: 10px;
	left: 12px;
}
`)
	clientLayout := qt.NewQFormLayout2()
	clientLayout.SetContentsMargins(10, 20, 10, 20)
	clientLayout.SetSpacing(15)
	clientLayout.SetLabelAlignment(qt.AlignRight)
	clientLayout.SetFieldGrowthPolicy(qt.QFormLayout__ExpandingFieldsGrow)
	clientGroup.SetLayout(clientLayout.QLayout)

	//mw.maxConcurrentSpin = qt.NewQSpinBox(clientGroup.QWidget)
	//mw.maxConcurrentSpin.SetRange(1, 20)
	//mw.maxConcurrentSpin.SetValue(5)
	//clientLayout.AddRow3("最大并发请求数:", mw.maxConcurrentSpin.QWidget)

	mw.onlyTranslateCJKCheck = qt.NewQCheckBox(clientGroup.QWidget)
	mw.onlyTranslateCJKCheck.SetChecked(true)
	clientLayout.AddRow3("仅翻译CJK文本:", mw.onlyTranslateCJKCheck.QWidget)

	mw.promptEdit = qt.NewQTextEdit(clientGroup.QWidget)
	mw.promptEdit.SetMaximumHeight(100)
	clientLayout.AddRow3("翻译提示词:", mw.promptEdit.QWidget)

	mainLayout.AddWidget(clientGroup.QWidget)

	mainLayout.AddStretch()

	return settingsPage
}

// selectInputFile 打开文件选择对话框，让用户选择要翻译的Excel文件
func (mw *MainWindow) selectInputFile() {
	startDir := mw.lastOpenDir
	if startDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil || homeDir == "" {
			homeDir = os.Getenv("HOME")
			if homeDir == "" {
				homeDir = os.Getenv("USERPROFILE")
			}
		}

		if homeDir != "" {
			downloadsDir := filepath.Join(homeDir, "Downloads")
			if info, err := os.Stat(downloadsDir); err == nil && info.IsDir() {
				startDir = downloadsDir
			} else {
				startDir = homeDir
			}
		}
	}

	fileName := qt.QFileDialog_GetOpenFileName4(
		mw.window.QWidget,
		"选择Excel文件",
		startDir,
		"Excel files (*.xlsx *.docx);;All Files (*)",
	)
	if fileName != "" {
		mw.inputFileEdit.SetText(fileName)
		mw.lastOpenDir = filepath.Dir(fileName)
		mw.logTextEdit.Clear()
		mw.resetProgressBar()
	}
}

// startTranslation 开始翻译过程，创建临时文件并在协程中执行翻译
// 使用mainthread.Wait确保UI更新在主线程中进行，避免界面卡死
func (mw *MainWindow) startTranslation() {
	// 使用互斥锁保护状态检查和设置
	mw.stateMutex.Lock()
	defer mw.stateMutex.Unlock()

	// 防止重复启动翻译
	if mw.isTranslating {
		mw.addLog("翻译正在进行中，请等待当前翻译完成")
		return
	}

	inputFile := mw.inputFileEdit.Text()

	if inputFile == "" {
		qt.QMessageBox_Warning(mw.window.QWidget, "错误", "请选择要翻译的文件")
		return
	}

	mw.resetProgressBar()
	mw.logTextEdit.Clear()

	tempDir := os.TempDir()
	base := filepath.Base(inputFile)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	tempFile := filepath.Join(tempDir, name+"_translated_"+fmt.Sprintf("%d", time.Now().Unix())+ext)
	mw.tempOutputFile = tempFile

	mw.isTranslating = true
	mw.updateButtonStates()

	mw.addLog("开始翻译...")

	mw.ctx, mw.cancel = context.WithCancel(context.Background())

	go func() {
		// 确保临时文件最终被清理
		defer func() {
			if mw.tempOutputFile != "" {
				if _, statErr := os.Stat(mw.tempOutputFile); statErr == nil {
					if removeErr := os.Remove(mw.tempOutputFile); removeErr != nil {
						log.Printf("清理临时文件失败: %v", removeErr)
					}
				}
			}
		}()

		handleComplete := func(err error) {
			mainthread.Wait(func() {
				// 使用互斥锁保护状态更新
				mw.stateMutex.Lock()
				defer mw.stateMutex.Unlock()

				if err != nil {
					var friendlyMsg string
					if errors.Is(err, context.Canceled) {
						friendlyMsg = "翻译已取消"
					} else if errors.Is(err, context.DeadlineExceeded) {
						friendlyMsg = "翻译超时，请检查网络连接或重试"
					} else {
						friendlyMsg = err.Error()
					}
					if mw.isTranslating {
						mw.finishTranslation(false)
					}
					mw.addLog(fmt.Sprintf("翻译失败: %s", friendlyMsg))
					if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						qt.QMessageBox_Critical(mw.window.QWidget, "错误", fmt.Sprintf("翻译失败: %s", friendlyMsg))
					}
				} else {
					if mw.isTranslating {
						mw.finishTranslation(true)
					}
					mw.addLog("翻译完成!")
					mw.promptSaveFile()
				}
			})
		}

		_ = runner.RunTranslation(mw.ctx, inputFile, tempFile, runner.TranslationCallbacks{
			OnTranslated: func(original, translated string) {
				mainthread.Wait(func() {
					mw.addLog(fmt.Sprintf("%s -> %s", original, translated))
				})
			},
			OnProgress: func(phase string, done, total int) {
				mainthread.Wait(func() {
					progress := done * 100 / total
					if progress > 100 {
						progress = 100
					}
					mw.progressBar.SetValue(progress)
				})
			},
			OnError: func(stage string, err error) {
				mainthread.Wait(func() {
					if errors.Is(err, context.Canceled) {
						return
					}
					if stage == "llm" {
						mw.addLog("翻译模型调用失败，请检查模型配置")
					} else {
						mw.addLog(fmt.Sprintf("翻译失败（阶段: %s）", stage))
					}
				})
			},
			OnComplete: handleComplete,
		})
	}()
}

// stopTranslation 停止当前翻译过程，取消上下文并恢复UI状态
func (mw *MainWindow) stopTranslation() {
	// 使用互斥锁保护状态检查和设置
	mw.stateMutex.Lock()
	defer mw.stateMutex.Unlock()

	if !mw.isTranslating {
		return
	}

	mw.addLog("用户停止翻译，正在清理资源...")

	// 立即设置状态，避免重复调用
	mw.finishTranslation(false)

	if mw.cancel != nil {
		mw.cancel()
		mw.addLog("翻译已停止")
	}
}

// updateButtonStates 根据当前翻译状态更新按钮的启用/禁用状态
func (mw *MainWindow) updateButtonStates() {
	if mw.isTranslating {
		mw.startBtn.SetEnabled(false)
		mw.stopBtn.SetEnabled(true)
	} else {
		mw.startBtn.SetEnabled(true)
		mw.stopBtn.SetEnabled(false)
	}
}

// resetProgressBar 重置进度条到初始状态
func (mw *MainWindow) resetProgressBar() {
	mw.progressBar.Reset()
	mw.progressBar.SetStyleSheet("")
}

// finishTranslation 完成翻译后的UI状态恢复，重新启用开始按钮并禁用停止按钮
func (mw *MainWindow) finishTranslation(success bool) {
	mw.isTranslating = false
	mw.updateButtonStates()
	mw.progressBar.SetValue(100)
	if success {
		mw.progressBar.SetStyleSheet(`
QProgressBar {
    background-color: #E6E6E6;
    margin-top: 1px;
    margin-bottom: 1px; 
}
QProgressBar::chunk { background-color: #4CAF50; border-radius: 3px; }
`)
	} else {
		mw.progressBar.SetStyleSheet(`
QProgressBar {
    background-color: #E6E6E6;
    margin-top: 1px;
    margin-bottom: 1px; 
}
QProgressBar::chunk { background-color: #F44336; border-radius: 3px; }
`)
	}
}

// addLog 添加日志到界面
func (mw *MainWindow) addLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logMessage := fmt.Sprintf("[%s] %s", timestamp, message)

	cursor := mw.logTextEdit.TextCursor()
	cursor.MovePosition(qt.QTextCursor__End)
	mw.logTextEdit.SetTextCursor(cursor)
	mw.logTextEdit.InsertPlainText(logMessage + "\n")

	mw.logTextEdit.EnsureCursorVisible()
}

// saveConfig 保存当前设置到配置文件
func (mw *MainWindow) saveConfig() {
	// Load existing config to preserve fields not shown in UI (like Log)
	cfg, err := config.Load()
	if err != nil {
		// If load fails, start with default config to ensure we have a valid structure
		// This might lose hidden settings if the file is corrupted, but it's safer than crashing
		cfg = config.DefaultConfig()
	}

	// Update fields from UI
	cfg.LLM.APIKey = mw.apiKeyEdit.Text()
	cfg.LLM.BaseURL = mw.apiUrlEdit.Text()
	cfg.LLM.Model = mw.modelEdit.Text()
	cfg.LLM.Prompt = mw.promptEdit.ToPlainText()
	cfg.Extractor.CJKOnly = mw.onlyTranslateCJKCheck.IsChecked()

	err = config.Save(cfg)
	if err != nil {
		qt.QMessageBox_Critical(mw.window.QWidget, "错误", fmt.Sprintf("保存配置失败: %v", err))
	} else {
		qt.QMessageBox_Information(mw.window.QWidget, "成功", "配置已保存")
	}
}

// promptSaveFile 翻译完成后提示用户保存翻译结果
// 自动生成默认文件名，并记住用户选择的保存目录
func (mw *MainWindow) promptSaveFile() {
	base := filepath.Base(mw.inputFileEdit.Text())
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	defaultName := name + "_译文" + ext

	startDir := mw.lastSaveDir
	if startDir == "" {
		startDir = mw.lastOpenDir
	}
	if startDir == "" {
		startDir = os.Getenv("HOME")
		if startDir == "" {
			startDir = os.Getenv("USERPROFILE")
		}
	}

	defaultPath := filepath.Join(startDir, defaultName)
	savePath := qt.QFileDialog_GetSaveFileName4(
		mw.window.QWidget,
		"保存翻译后的文件",
		defaultPath,
		"Excel files (*.xlsx *.docx);;All Files (*)",
	)

	if savePath != "" {
		mw.lastSaveDir = filepath.Dir(savePath)

		err := copyFile(mw.tempOutputFile, savePath)
		if err != nil {
			qt.QMessageBox_Critical(mw.window.QWidget, "错误", fmt.Sprintf("保存文件失败: %v", err))
			return
		}

		qt.QMessageBox_Information(mw.window.QWidget, "成功", fmt.Sprintf("文件已保存到: %s", savePath))
	} else {
		qt.QMessageBox_Information(mw.window.QWidget, "完成", "翻译已完成，但未保存文件。\n临时文件位置: "+mw.tempOutputFile)
	}
}

// copyFile 复制文件的工具函数
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// createMenuBar 创建应用程序菜单栏，包含偏好设置菜单
func (mw *MainWindow) createMenuBar() {
	menuBar := qt.NewQMenuBar2()
	mw.window.SetMenuBar(menuBar)
	appMenu := menuBar.AddMenuWithTitle("File")
	preferencesAction := qt.NewQAction2("Preferences...")
	preferencesAction.SetMenuRole(qt.QAction__PreferencesRole)
	preferencesAction.SetShortcutsWithShortcuts(qt.QKeySequence__Preferences)
	preferencesAction.OnTriggered(func() {
		mw.showSettingsWindow()
	})
	appMenu.AddAction(preferencesAction)
}

// showSettingsWindow 显示设置对话框，允许用户配置API参数和翻译选项
func (mw *MainWindow) showSettingsWindow() {
	settingsWindow := qt.NewQDialog(mw.window.QWidget)
	settingsWindow.SetWindowTitle("偏好设置")
	settingsWindow.SetModal(true)
	settingsWindow.SetMinimumSize(qt.NewQSize2(500, 400))

	layout := qt.NewQVBoxLayout2()
	settingsWindow.SetLayout(layout.QBoxLayout.QLayout)

	settingsWidget := mw.createSettingsPage()
	layout.AddWidget(settingsWidget)

	mw.loadConfigToSettings()

	buttonLayout := qt.NewQHBoxLayout2()
	buttonLayout.SetSpacing(20)
	buttonLayout.AddStretch()

	saveBtn := qt.NewQPushButton3("保存")
	saveBtn.SetFixedWidth(80)
	saveBtn.OnPressed(func() {
		mw.saveConfig()
		settingsWindow.Accept()
	})
	buttonLayout.AddWidget(saveBtn.QWidget)

	cancelBtn := qt.NewQPushButton3("取消")
	cancelBtn.SetFixedWidth(80)
	cancelBtn.OnPressed(func() {
		settingsWindow.Reject()
	})
	buttonLayout.AddWidget(cancelBtn.QWidget)

	buttonLayout.AddStretch()

	layout.AddLayout(buttonLayout.QBoxLayout.QLayout)

	settingsWindow.Show()
	settingsWindow.Exec()
}

// setupDragAndDrop 设置文件拖拽功能，支持将Excel文件拖拽到文件选择区域
func (mw *MainWindow) setupDragAndDrop() {
	mw.fileGroup.OnDragEnterEvent(func(super func(event *qt.QDragEnterEvent), event *qt.QDragEnterEvent) {
		if event.MimeData().HasUrls() {
			event.AcceptProposedAction()
		} else {
			super(event)
		}
	})

	mw.fileGroup.OnDragMoveEvent(func(super func(event *qt.QDragMoveEvent), event *qt.QDragMoveEvent) {
		if event.MimeData().HasUrls() {
			event.AcceptProposedAction()
		} else {
			super(event)
		}
	})

	mw.fileGroup.OnDropEvent(func(super func(event *qt.QDropEvent), event *qt.QDropEvent) {
		mimeData := event.MimeData()
		if mimeData.HasUrls() {
			urls := mimeData.Urls()
			if len(urls) > 0 {
				filePath := urls[0].ToLocalFile()

				ext := strings.ToLower(filepath.Ext(filePath))
				if ext == ".xlsx" || ext == ".docx" {
					mw.inputFileEdit.SetText(filePath)
					mw.lastOpenDir = filepath.Dir(filePath)
					mw.logTextEdit.Clear()
					mw.resetProgressBar()
					event.AcceptProposedAction()
				} else {
					qt.QMessageBox_Warning(mw.window.QWidget, "错误", "请拖拽Excel文件(.xlsx或.docx)")
				}
			}
		} else {
			super(event)
		}
	})
}

// loadConfigToSettings 从配置文件加载设置到UI组件
func (mw *MainWindow) loadConfigToSettings() {
	cfg, err := config.Load() // Change to config.Load
	if err != nil {
		qt.QMessageBox_Warning(mw.window.QWidget, "警告", fmt.Sprintf("加载配置失败: %v", err))
		return
	}

	mw.apiKeyEdit.SetText(cfg.LLM.APIKey)
	mw.apiUrlEdit.SetText(cfg.LLM.BaseURL) // Note: APIURL in GUI maps to BaseURL in config
	mw.modelEdit.SetText(cfg.LLM.Model)
	mw.promptEdit.SetText(cfg.LLM.Prompt) // Map LLM.Prompt directly
	// mw.maxConcurrentSpin.SetValue(cfg.Client.MaxConcurrentRequests) // No direct mapping in AppConfig
	mw.onlyTranslateCJKCheck.SetChecked(cfg.Extractor.CJKOnly) // Map Extractor.CJKOnly
}

// main 函数是程序的入口点
func main() {
	qt.NewQApplication(os.Args)

	// 设置应用程序图标（在 Windows 上显示在标题栏和任务栏）
	pixmap := qt.NewQPixmap()
	if pixmap.LoadFromDataWithData(appIconData) {
		icon := qt.NewQIcon2(pixmap)
		qt.QGuiApplication_SetWindowIcon(icon)
	}

	window := NewMainWindow()
	window.window.Show()

	qt.QApplication_Exec()
}
