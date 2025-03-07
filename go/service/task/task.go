package task

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nooncall/owls/go/model/common/request"
	"github.com/nooncall/owls/go/utils/logger"
)

// 新建task 模块，用以封装统一的task、审批流

// 1, 设计数据结构
// 2, 权限，也使用新的task
// 3, todo， 改写 审批模块，使用新的task； 实现功能为主，代码优化功能强大后再做。
// 4, 暂时先不管审批task的流程，直接新做一套新的，

// todo, 接口层，先用一个参数指定类型，set 接口filed，再unmarshel

type Task struct {
	ID               int64  `json:"id" gorm:"column:id"`
	Name             string `json:"name" gorm:"column:name"`
	Status           string `json:"status" gorm:"column:status"`
	Creator          string `json:"creator" gorm:"column:creator"`
	Reviewer         string `json:"reviewer" gorm:"column:reviewer"`
	Executor         string `json:"executor" gorm:"column:executor"`
	Description      string `json:"description"`
	ExecInfo         string `json:"exec_info" gorm:"column:exec_info"`
	SubTaskID        int64  `json:"sub_task_id"`
	SubTaskType      string `json:"sub_task_type"`
	RejectContent    string `json:"reject_content" gorm:"column:reject_content"`
	Cluster          string `json:"cluster" gorm:"column:cluster"`
	Database         string `json:"database" gorm:"column:database"`
	ContinueOnFailed bool   `json:"continue_on_failed" gorm:"column:continue_on_failed"`
	Ct               int64  `json:"ct" gorm:"column:ct"`
	Ut               int64  `json:"ut" gorm:"column:ut"`
	Et               int64  `json:"et" gorm:"column:et"`
	Ft               int64  `json:"ft" gorm:"column:ft"`

	SubTask   SubTask   `json:"sub_task" gorm:"-"`
	SubTasks  []SubTask `json:"sub_tasks" gorm:"-"`
	StartAtId int64     `json:"start_at_id" gorm:"-"`

	StatusName string `json:"status_name" gorm:"-"`
	Action     string `json:"action" gorm:"-"`
}

type SubTask interface {
	AddTask(ctx context.Context, cluster string, db int, parentTaskID int64) (int64, bool, error)
	//TODO, refactor, auth db on subtask, redis db on parent task;
	ExecTask(ctx context.Context, startSubTaskId, taskId int64, cluster, db string) error
	UpdateTask(action string) error
	ListTask(parentTaskID int64) (interface{}, error)
	GetTask(id int64) (interface{}, error)
}

func AddTask(ctx context.Context, task *Task) (int64, error) {
	task.Ct = time.Now().Unix()
	task.Status = WaitApproval
	taskId, err := taskDao.AddTask(task)
	if err != nil {
		return 0, err
	}

	db, err := strconv.Atoi(task.Database)
	if err != nil {
		return 0, err
	}

	subTaskID, pass, err := task.SubTask.AddTask(ctx, task.Cluster, db, taskId)
	if err != nil {
		return 0, err
	}
	if !pass {
		task.Status = Reject
	}

	task.SubTaskID, task.ID = subTaskID, taskId
	if err = taskDao.UpdateTask(task); err != nil {
		logger.Errorf("update task with subtask err: %v", err)
	}

	return taskId, err
}

func UpdateTask(ctx context.Context, task *Task, startAtId int64) error {
	// subtask is nil
	var err error
	if err = task.SubTask.UpdateTask(task.Action); err != nil {
		return err
	}

	switch task.Action {
	case ActionCancel:
		task.Status = Cancel
	case ActionApproval:
		task.Status = Pass
	case ActionReject:
		task.Status = Reject
	case ActionResubmit:
		task.Status = WaitApproval
	case ActionExec:
		storeTask, err := GetTask(task.ID, "", task.SubTask)
		if err != nil {
			return err
		}
		task.Status = Pass
		task.SubTask.ExecTask(ctx, startAtId, storeTask.ID, storeTask.Cluster, storeTask.Database)
	case ActionUpdate:
	}

	return taskDao.UpdateTask(task)
}

func ListTask(pageInfo request.SortPageInfo, status []string, subTask SubTask, subType string) (interface{}, int64, error) {
	tasks, count, err := taskDao.ListTask(pageInfo, true, status, subType)
	if err != nil {
		return nil, 0, err
	}

	for i := range tasks {
		if err = getSubTask(&tasks[i], subTask); err != nil {
			logger.Warnf("get sub task error: %v", err)
		}
	}

	return tasks, count, nil
}

func GetTask(id int64, operator string, subTask SubTask) (*Task, error) {
	task, err := taskDao.GetTask(id)
	if err != nil {
		return nil, err
	}

	err = getSubTask(task, subTask)
	return task, nil
}

func GetOnlyTask(id int64) (*Task, error) {
	return taskDao.GetTask(id)
}

func getSubTask(task *Task, subTask SubTask) error {
	var err error

	switch task.SubTaskType {
	case Auth:
		subTaskNoType, err := subTask.GetTask(task.SubTaskID)
		if err != nil {
			logger.Errorf("get sub task error: %v", err)
			return nil
		}
		task.SubTask = subTaskNoType.(SubTask)
	case Redis:
		subTasks, err := subTask.ListTask(task.ID)
		if err != nil {
			logger.Errorf("get sub task for redis error: %v", err)
			return nil
		}

		task.SubTasks = subTasks.([]SubTask)
	default:
		return fmt.Errorf("sub task type err: %s", task.SubTaskType)
	}

	return fmt.Errorf("get subTask for %d err: %v", task.SubTaskID, err)
}
